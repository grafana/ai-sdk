package anthropic

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/grafana/ai-sdk/provider"
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
	ping := `{"type":"ping"}`
	largePing := `{"type":"ping","padding":"` + strings.Repeat("a", (maxPreflightBytes/32)-1-len(`{"type":"ping","padding":""}`)) + `"}`
	for _, tc := range []struct {
		name           string
		frame          string
		count          int
		overflow       bool
		initialError   bool
		oversizedError bool
	}{
		{name: "frame boundary", frame: ping, count: maxPreflightFrames - 1},
		{name: "frame overflow", frame: ping, count: maxPreflightFrames, overflow: true},
		{name: "byte boundary", frame: largePing, count: 31},
		{name: "byte overflow", frame: largePing, count: 33, overflow: true},
		{name: "initial error", frame: ping, count: 2, initialError: true},
		{name: "error frame overflow", frame: ping, count: maxPreflightFrames, overflow: true, initialError: true},
		{name: "error byte overflow", overflow: true, initialError: true, oversizedError: true},
	} {
		for _, raw := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/raw=%t", tc.name, raw), func(t *testing.T) {
				events := make([]string, tc.count)
				for i := range events {
					events[i] = tc.frame
				}
				if tc.initialError {
					events = append(events, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`)
				} else {
					handoff := transportEvents[0]
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
					payload = "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"},\"padding\":[\n" + strings.Join(chunks, "\n") + "\ndata: ]}\n\n"
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
				m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
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
						assert.Equal(t, 529, apiErr.StatusCode)
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

func TestDoStream_IgnoredFramesPreserveSDKBehavior(t *testing.T) {
	for _, eventType := range []string{"ping", "future.event", ""} {
		for _, before := range []bool{false, true} {
			t.Run(fmt.Sprintf("event=%s/before=%t", eventType, before), func(t *testing.T) {
				ignored := fmt.Sprintf("event: %s\ndata: not JSON\n\n", eventType)
				payload := transportSSE(transportEvents[:1]) + ignored + transportSSE(transportEvents[1:])
				if before {
					payload = ignored + transportSSE(transportEvents)
				}
				sdkStream := ssestream.NewStream[sdk.BetaRawMessageStreamEventUnion](ssestream.NewDecoder(&http.Response{Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(payload))}), nil)
				defer func() { _ = sdkStream.Close() }()
				var events int
				for sdkStream.Next() {
					events++
				}
				require.NoError(t, sdkStream.Err())
				require.Equal(t, len(transportEvents), events)
				for _, raw := range []bool{false, true} {
					m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
						response := transportResponse(r, true)
						response.Body = io.NopCloser(strings.NewReader(payload))
						return response, nil
					})})))
					result, err := m.DoStream(t.Context(), provider.CallOptions{IncludeRawChunks: raw})
					require.NoError(t, err)
					var text string
					var nilRaw int
					for part := range result.Stream {
						assert.NotEqual(t, provider.PartError, part.Type)
						if part.Type == provider.PartTextDelta {
							text += part.Delta
						}
						if part.Type == provider.PartRaw && part.RawValue == nil {
							nilRaw++
						}
					}
					assert.Equal(t, "hello", text)
					if raw {
						assert.Equal(t, 1, nilRaw)
					} else {
						assert.Zero(t, nilRaw)
					}
				}
			})
		}
	}
}
