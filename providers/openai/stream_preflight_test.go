package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type startupBody struct {
	io.ReadCloser
	closes atomic.Int32
	closed chan struct{}
}

func (b *startupBody) Close() error {
	count := b.closes.Add(1)
	err := b.ReadCloser.Close()
	if count == 1 && b.closed != nil {
		close(b.closed)
	}
	return err
}

func TestDoStream_StartupBudget(t *testing.T) {
	ignored := `{"type":"unknown_chunk"}`
	large := `{"type":"unknown_chunk","padding":"` + strings.Repeat("a", (maxPreflightBytes/32)-1-len(`{"type":"unknown_chunk","padding":""}`)) + `"}`
	for _, tc := range []struct {
		name           string
		frame          string
		count          int
		overflow       bool
		initialError   bool
		oversizedError bool
	}{
		{name: "frame boundary", frame: ignored, count: maxPreflightFrames - 1},
		{name: "frame overflow", frame: ignored, count: maxPreflightFrames, overflow: true},
		{name: "byte boundary", frame: large, count: 31},
		{name: "byte overflow", frame: large, count: 33, overflow: true},
		{name: "initial error", frame: ignored, count: 2, initialError: true},
		{name: "error frame overflow", frame: ignored, count: maxPreflightFrames, overflow: true, initialError: true},
		{name: "error byte overflow", overflow: true, initialError: true, oversizedError: true},
	} {
		for _, raw := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/raw=%t", tc.name, raw), func(t *testing.T) {
				events := make([]string, tc.count)
				for i := range events {
					events[i] = tc.frame
				}
				if tc.initialError {
					events = append(events, `{"type":"error","code":"rate_limit_exceeded","message":"busy"}`)
				} else {
					handoff := transportEvents[1]
					if tc.name == "byte boundary" {
						prefix := strings.TrimSuffix(handoff, "}") + `,"padding":"`
						handoff = prefix + strings.Repeat("a", (maxPreflightBytes/32)-1-len(prefix)-2) + `"}`
					}
					events = append(events, handoff)
				}
				payload := transportSSE(events)
				if tc.oversizedError {
					chunks := make([]string, 33)
					for i := range chunks {
						chunks[i] = `data: "` + strings.Repeat("a", 32*1024) + `"`
						if i < len(chunks)-1 {
							chunks[i] += ","
						}
					}
					payload = "data: {\"type\":\"error\",\"code\":\"rate_limit_exceeded\",\"message\":\"busy\",\"padding\":[\n" + strings.Join(chunks, "\n") + "\ndata: ]}\n\n"
				}
				body := &startupBody{ReadCloser: io.NopCloser(strings.NewReader(payload)), closed: make(chan struct{})}
				if tc.overflow {
					reader, writer := io.Pipe()
					body.ReadCloser = reader
					go func() {
						defer func() { _ = writer.Close() }()
						_, _ = io.WriteString(writer, payload)
						<-body.closed
					}()
				}
				m := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					response := transportResponse(r, true)
					response.Body = body
					return response, nil
				})})))
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				result, err := m.DoStream(ctx, provider.CallOptions{IncludeRawChunks: raw})
				if tc.overflow || tc.initialError {
					require.Error(t, err)
					assert.Nil(t, result)
					var apiErr *provider.APICallError
					require.ErrorAs(t, err, &apiErr)
					if tc.overflow {
						assert.Contains(t, err.Error(), "startup buffer limit exceeded")
						assert.False(t, apiErr.IsRetryable)
					} else {
						assert.Equal(t, 429, apiErr.StatusCode)
					}
				} else {
					require.NoError(t, err)
					for range result.Stream {
					}
				}
				require.Eventually(t, func() bool { return body.closes.Load() == 1 }, time.Second, time.Millisecond)
			})
		}
	}
}

func TestDoStream_RawInitialError(t *testing.T) {
	m := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		response := transportResponse(r, true)
		response.Body = io.NopCloser(strings.NewReader(transportSSE([]string{`{"type":"error","code":"insufficient_quota","message":"failed"}`})))
		return response, nil
	})})))
	result, err := m.DoStream(t.Context(), provider.CallOptions{IncludeRawChunks: true})
	require.Error(t, err)
	assert.Nil(t, result)
	var apiErr *provider.APICallError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
}

func TestPreflightResponseStream(t *testing.T) {
	requestBody := responses.ResponseNewParams{Model: "gpt-4o"}

	t.Run("pre-cancelled context wins over closed stream", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		items := make(chan responseStreamItem)
		close(items)

		buffered, err := preflightResponseStream(ctx, items, requestBody, nil)
		assert.Nil(t, buffered)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("cancellation while waiting is returned directly", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		items := make(chan responseStreamItem)
		result := make(chan error, 1)
		go func() {
			_, err := preflightResponseStream(ctx, items, requestBody, nil)
			result <- err
		}()
		cancel()

		assert.ErrorIs(t, <-result, context.Canceled)
		close(items)
	})

	t.Run("error flushed with in progress is returned", func(t *testing.T) {
		items := make(chan responseStreamItem, 2)
		accepted := unmarshalEvent(t, `{"type":"response.in_progress","sequence_number":1,"response":{"id":"resp_1","status":"in_progress"}}`)
		failed := unmarshalEvent(t, `{"type":"error","sequence_number":2,"message":"quota exhausted","code":"insufficient_quota"}`)
		items <- responseStreamItem{event: &accepted}
		items <- responseStreamItem{event: &failed}
		close(items)

		buffered, err := preflightResponseStream(context.Background(), items, requestBody, nil)
		assert.Nil(t, buffered)
		require.Error(t, err)
		var apiErr *provider.APICallError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, 429, apiErr.StatusCode)
		assert.True(t, apiErr.IsRetryable)
		assert.JSONEq(t, `{"type":"error","sequence_number":2,"message":"quota exhausted","code":"insufficient_quota"}`, apiErr.ResponseBody)
		assert.NotEmpty(t, apiErr.RequestBodyValues)
	})

	t.Run("returns after accepted grace and surfaces later error in stream", func(t *testing.T) {
		items := make(chan responseStreamItem, 2)
		accepted := unmarshalEvent(t, `{"type":"response.in_progress","sequence_number":1,"response":{"id":"resp_1","status":"in_progress"}}`)
		failed := unmarshalEvent(t, `{"type":"error","sequence_number":2,"message":"late failure","code":"rate_limit_error"}`)
		items <- responseStreamItem{event: &accepted}
		go func() {
			time.Sleep(acceptedStreamErrorGrace + 25*time.Millisecond)
			items <- responseStreamItem{event: &failed}
			close(items)
		}()

		buffered, err := preflightResponseStream(context.Background(), items, requestBody, nil)
		require.NoError(t, err)
		require.Len(t, buffered, 1)

		parts := make(chan provider.StreamPart, 8)
		go func() {
			defer close(parts)
			consumeStream(context.Background(), items, buffered, parts, nil, buildResult{}, requestBody, nil, seqIDGen(), "openai")
		}()
		var got []provider.StreamPart
		for part := range parts {
			got = append(got, part)
		}
		require.Len(t, got, 3)
		assert.Equal(t, provider.PartStreamStart, got[0].Type)
		assert.Equal(t, provider.PartError, got[1].Type)
		require.NotNil(t, got[1].APICallError)
		assert.Equal(t, 429, got[1].APICallError.StatusCode)
		assert.Equal(t, provider.PartFinish, got[2].Type)
		require.NotNil(t, got[2].FinishReason)
		assert.Equal(t, provider.FinishReasonError, got[2].FinishReason.Unified)
	})

	t.Run("response failed returns structured error", func(t *testing.T) {
		items := make(chan responseStreamItem, 1)
		failed := unmarshalEvent(t, `{"type":"response.failed","sequence_number":1,"response":{"id":"resp_1","status":"failed","error":{"message":"bad input","code":"invalid_request_error"}}}`)
		items <- responseStreamItem{event: &failed}
		close(items)

		_, err := preflightResponseStream(context.Background(), items, requestBody, nil)
		require.Error(t, err)
		var apiErr *provider.APICallError
		require.True(t, errors.As(err, &apiErr))
		assert.Equal(t, 400, apiErr.StatusCode)
		assert.Equal(t, "bad input", apiErr.Message)
	})

	t.Run("first output returns immediately with buffered events", func(t *testing.T) {
		items := make(chan responseStreamItem, 2)
		created := unmarshalEvent(t, `{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","status":"in_progress"}}`)
		output := unmarshalEvent(t, `{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`)
		items <- responseStreamItem{event: &created}
		items <- responseStreamItem{event: &output}
		close(items)

		buffered, err := preflightResponseStream(context.Background(), items, requestBody, nil)
		require.NoError(t, err)
		require.Len(t, buffered, 2)
	})
}
