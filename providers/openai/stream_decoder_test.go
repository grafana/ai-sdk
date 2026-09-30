package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingStreamDecoder struct {
	ssestream.Decoder
	closes atomic.Int32
	closed chan struct{}
	once   sync.Once
}

func (d *countingStreamDecoder) Close() error {
	d.closes.Add(1)
	defer d.once.Do(func() { close(d.closed) })
	return d.Decoder.Close()
}

func TestDoStream_CancelWithoutConsumer(t *testing.T) {
	reader, writer := io.Pipe()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer func() { _ = writer.Close() }()
		for range 512 {
			if _, err := io.WriteString(writer, transportSSE([]string{transportEvents[1]})); err != nil {
				return
			}
		}
	}()
	m := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		response := transportResponse(r, true)
		response.Body = reader
		return response, nil
	})})))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := m.DoStream(ctx, provider.CallOptions{IncludeRawChunks: true})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(result.Stream) == cap(result.Stream) }, time.Second, time.Millisecond)
	cancel()
	select {
	case <-writerDone:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "cancel did not close the response body")
	}
}

func TestConsumeStream_CancelStalledConsumer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	events := append([]string(nil), transportEvents[:1]...)
	for range 512 {
		events = append(events, transportEvents[1])
	}
	body := &startupBody{ReadCloser: io.NopCloser(strings.NewReader(transportSSE(events)))}
	response := &http.Response{Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
	items := pumpResponseStream(ctx, response, nil, true)
	output := make(chan provider.StreamPart, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		consumeStream(ctx, items, nil, output, nil, buildResult{}, responses.ResponseNewParams{}, response, func() string { return "id" }, "openai")
	}()
	require.Eventually(t, func() bool { return len(output) == cap(output) }, time.Second, time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		require.FailNow(t, "producer required output reads after cancellation")
	}
	require.Eventually(t, func() bool { return body.closes.Load() == 1 }, time.Second, time.Millisecond)
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

func TestDoStream_SDKDecoderOwnership(t *testing.T) {
	for _, cancelStream := range []bool{false, true} {
		name := "completed"
		if cancelStream {
			name = "cancelled"
		}
		t.Run(name, func(t *testing.T) {
			contentType := "application/x-aisdk-decoder-" + name
			var mu sync.Mutex
			var decoders []*countingStreamDecoder
			ssestream.RegisterDecoder(contentType, func(body io.ReadCloser) ssestream.Decoder {
				var marker [1]byte
				_, err := io.ReadFull(body, marker[:])
				assert.NoError(t, err)
				assert.Equal(t, byte('!'), marker[0])
				d := &countingStreamDecoder{Decoder: ssestream.NewDecoder(&http.Response{Body: body, Header: http.Header{"Content-Type": []string{"text/event-stream"}}}), closed: make(chan struct{})}
				mu.Lock()
				decoders = append(decoders, d)
				mu.Unlock()
				return d
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/responses", r.URL.Path)
				assert.Equal(t, "Bearer sdk-key", r.Header.Get("Authorization"))
				assert.Equal(t, "client", r.Header.Get("X-SDK-Client"))
				assert.Equal(t, "call", r.Header.Get("X-SDK-Call"))
				var body map[string]any
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				assert.Equal(t, true, body["stream"])
				w.Header().Set("Content-Type", contentType)
				_, err := io.WriteString(w, "!data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"delta\":\"hello\"}\n\n")
				assert.NoError(t, err)
				if cancelStream {
					assert.NoError(t, http.NewResponseController(w).Flush())
					<-r.Context().Done()
					return
				}
				_, err = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output\":[]}}\n\ndata: [DONE]\n\n")
				assert.NoError(t, err)
			}))
			defer server.Close()
			client := openaisdk.NewClient(option.WithAPIKey("sdk-key"), option.WithBaseURL(server.URL), option.WithHeader("X-SDK-Client", "client"), option.WithMaxRetries(0))
			model := NewResponsesWithClient(client, "gpt-4o")
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			result, err := model.DoStream(ctx, provider.CallOptions{Headers: map[string]string{"X-SDK-Call": "call"}, IncludeRawChunks: true})
			require.NoError(t, err)
			var text string
			for part := range result.Stream {
				if part.Type == provider.PartTextDelta {
					text += part.Delta
					if cancelStream {
						cancel()
					}
				}
			}
			assert.Equal(t, "hello", text)
			mu.Lock()
			instances := append([]*countingStreamDecoder(nil), decoders...)
			mu.Unlock()
			require.Len(t, instances, 1)
			select {
			case <-instances[0].closed:
			case <-time.After(5 * time.Second):
				require.FailNow(t, "decoder was not closed")
			}
			assert.Equal(t, int32(1), instances[0].closes.Load())
		})
	}
}
