package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
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

func TestDoStream_WhitespaceData(t *testing.T) {
	for _, data := range []string{"   ", ""} {
		for _, before := range []bool{false, true} {
			for _, raw := range []bool{false, true} {
				t.Run(fmt.Sprintf("data=%q/before=%t/raw=%t", data, before, raw), func(t *testing.T) {
					frame := fmt.Sprintf("data: %s\n\n", data)
					payload := transportSSE(transportEvents[:2]) + frame + transportSSE(transportEvents[2:])
					if before {
						payload = frame + transportSSE(transportEvents)
					}
					m := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
						response := transportResponse(r, true)
						response.Body = io.NopCloser(strings.NewReader(payload))
						return response, nil
					})})))
					result, err := m.DoStream(t.Context(), provider.CallOptions{IncludeRawChunks: raw})
					require.NoError(t, err)
					var last provider.StreamPart
					var errors, nilRaws int
					for part := range result.Stream {
						if part.Type == provider.PartRaw && part.RawValue == nil {
							nilRaws++
						}
						if part.Type == provider.PartError {
							errors++
							if raw {
								assert.Equal(t, provider.PartRaw, last.Type)
								assert.Nil(t, last.RawValue)
							}
						}
						last = part
					}
					assert.Equal(t, 1, errors)
					if raw {
						assert.Equal(t, 1, nilRaws)
					} else {
						assert.Zero(t, nilRaws)
					}
				})
			}
		}
	}
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
