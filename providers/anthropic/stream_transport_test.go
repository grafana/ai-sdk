package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

type countingMessageDecoder struct {
	ssestream.Decoder
	closes atomic.Int32
	closed chan struct{}
	once   sync.Once
}

func (d *countingMessageDecoder) Close() error {
	d.closes.Add(1)
	defer d.once.Do(func() { close(d.closed) })
	return d.Decoder.Close()
}

func TestDoStream_SDKDecoderOwnership(t *testing.T) {
	for _, cancelStream := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%t", cancelStream), func(t *testing.T) {
			contentType := fmt.Sprintf("application/x-aisdk-anthropic-decoder-%t", cancelStream)
			instances := make(chan *countingMessageDecoder, 2)
			ssestream.RegisterDecoder(contentType, func(body io.ReadCloser) ssestream.Decoder {
				var marker [1]byte
				_, err := io.ReadFull(body, marker[:])
				assert.NoError(t, err)
				assert.Equal(t, byte('!'), marker[0])
				d := &countingMessageDecoder{Decoder: ssestream.NewDecoder(&http.Response{Body: body, Header: http.Header{"Content-Type": {"text/event-stream"}}}), closed: make(chan struct{})}
				instances <- d
				return d
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", contentType)
				_, _ = io.WriteString(w, "!"+transportSSE(transportEvents))
				if cancelStream {
					for range 512 {
						_, _ = io.WriteString(w, transportSSE([]string{transportEvents[2]}))
					}
					_ = http.NewResponseController(w).Flush()
					<-r.Context().Done()
				}
			}))
			defer server.Close()
			m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			result, err := m.DoStream(ctx, provider.CallOptions{IncludeRawChunks: true})
			require.NoError(t, err)
			instance := <-instances
			if cancelStream {
				require.Eventually(t, func() bool { return len(result.Stream) == cap(result.Stream) }, time.Second, time.Millisecond)
				cancel()
			} else {
				for range result.Stream {
				}
			}
			select {
			case <-instance.closed:
			case <-time.After(5 * time.Second):
				require.FailNow(t, "decoder was not closed")
			}
			assert.Equal(t, int32(1), instance.closes.Load())
			assert.Empty(t, instances)
		})
	}
}

func TestConsumeStream_CancelStalledConsumer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	events := append([]string(nil), transportEvents...)
	for range 512 {
		events = append(events, transportEvents[2])
	}
	body := &startupBody{ReadCloser: io.NopCloser(strings.NewReader(transportSSE(events)))}
	response := &http.Response{Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
	items := pumpMessageStream(ctx, response, true)
	output := make(chan provider.StreamPart, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		consumeStream(ctx, items, nil, output, toolNameMapping{}, nil, false, nil, func() string { return "id" }, "anthropic", false, nil)
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

func TestDoStream_RawTransportEvents(t *testing.T) {
	for _, tc := range []struct {
		name       string
		events     []string
		raw        []string
		errorFrame bool
	}{
		{name: "normal", events: transportEvents, raw: transportEvents},
		{name: "ping and ignored", events: []string{`{"type":"ping"}`, transportEvents[0], `{"type":"future.event","extra":42}`, transportEvents[4], transportEvents[5]}, raw: []string{`{"type":"ping"}`, transportEvents[0], `{"type":"future.event","extra":42}`, transportEvents[4], transportEvents[5]}},
		{name: "invalid JSON", events: []string{transportEvents[0], `not JSON`}, raw: []string{transportEvents[0], ""}, errorFrame: true},
		{name: "permissive SDK decoding", events: []string{transportEvents[0], `{"type":42}`}, raw: []string{transportEvents[0], `{"type":42}`}},
		{name: "normalization failure", events: []string{transportEvents[0], `{"type":"message_delta","delta":{"stop_reason":"end_turn","safeguard_results":[{"type":"dangerous_tool_use","status":{"type":"available","tool_uses":{"toolu_01":{"type":1,"explanation":"private"}}}}]},"usage":{"output_tokens":1}}`}, raw: []string{transportEvents[0], `{"type":"message_delta","delta":{"stop_reason":"end_turn","safeguard_results":[{"type":"dangerous_tool_use","status":{"type":"available","tool_uses":{"toolu_01":{"type":1,"explanation":"private"}}}}]},"usage":{"output_tokens":1}}`}, errorFrame: true},
		{name: "null", events: []string{transportEvents[0], `null`}, raw: []string{transportEvents[0], `null`}},
		{name: "error", events: []string{transportEvents[0], `{"type":"error","error":{"message":"failed","type":"api_error"}}`}, raw: []string{transportEvents[0], `{"type":"error","error":{"message":"failed","type":"api_error"}}`}, errorFrame: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var normalizedJSON []byte
			for _, includeRaw := range []bool{false, true} {
				m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
					response := transportResponse(r, true)
					response.Body = io.NopCloser(strings.NewReader(": comment\n\n" + transportSSE(tc.events) + "data: [DONE]\n\n"))
					return response, nil
				})})))
				result, err := m.DoStream(t.Context(), provider.CallOptions{IncludeRawChunks: includeRaw})
				require.NoError(t, err)
				var raws []json.RawMessage
				var parts, normalized []provider.StreamPart
				var errorCount int
				var latestRaw json.RawMessage
				for part := range result.Stream {
					if includeRaw && tc.name == "normal" && part.Type != provider.PartRaw {
						if want := map[provider.StreamPartType]string{provider.PartResponseMeta: transportEvents[0], provider.PartTextStart: transportEvents[1], provider.PartTextDelta: transportEvents[2], provider.PartTextEnd: transportEvents[3], provider.PartFinish: transportEvents[4]}[part.Type]; want != "" {
							assert.JSONEq(t, want, string(latestRaw))
						}
					}
					parts = append(parts, part)
					if part.Type == provider.PartRaw {
						raws = append(raws, part.RawValue)
						latestRaw = part.RawValue
					} else {
						normalized = append(normalized, part)
					}
					if part.Type == provider.PartError {
						errorCount++
					}
					if includeRaw && tc.errorFrame && part.Type == provider.PartError {
						require.GreaterOrEqual(t, len(parts), 2)
						assert.Equal(t, provider.PartRaw, parts[len(parts)-2].Type)
					}
				}
				if tc.errorFrame {
					assert.Positive(t, errorCount)
				}
				encoded, err := json.Marshal(normalized)
				require.NoError(t, err)
				if !includeRaw {
					normalizedJSON = encoded
				} else {
					assert.JSONEq(t, string(normalizedJSON), string(encoded))
				}
				require.NotEmpty(t, parts)
				assert.Equal(t, provider.PartStreamStart, parts[0].Type)
				if !includeRaw {
					assert.Empty(t, raws)
					continue
				}
				require.Len(t, raws, len(tc.raw))
				for i, want := range tc.raw {
					if want == "" {
						assert.Nil(t, raws[i])
					} else {
						assert.JSONEq(t, want, string(raws[i]))
					}
				}
			}
		})
	}
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

func TestPumpMessageStream_EmptyEligibleFrame(t *testing.T) {
	response := &http.Response{Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("event: message_delta\n\n"))}
	items := pumpMessageStream(t.Context(), response, true)
	item, ok := <-items
	require.True(t, ok)
	require.Error(t, item.err)
	assert.False(t, item.hasRaw)
	_, ok = <-items
	assert.False(t, ok)
}

func TestDoStream_WhitespaceData(t *testing.T) {
	for _, eventType := range []string{"message_delta", "ping"} {
		for _, data := range []string{"   ", ""} {
			for _, before := range []bool{false, true} {
				for _, raw := range []bool{false, true} {
					t.Run(fmt.Sprintf("event=%s/data=%q/before=%t/raw=%t", eventType, data, before, raw), func(t *testing.T) {
						frame := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
						payload := transportSSE(transportEvents[:1]) + frame + transportSSE(transportEvents[1:])
						if before {
							payload = frame + transportSSE(transportEvents)
						}
						m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
							response := transportResponse(r, true)
							response.Body = io.NopCloser(strings.NewReader(payload))
							return response, nil
						})})))
						result, err := m.DoStream(t.Context(), provider.CallOptions{IncludeRawChunks: raw})
						if eventType == "message_delta" && before {
							require.Error(t, err)
							assert.Nil(t, result)
							return
						}
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
						if eventType == "message_delta" {
							assert.Equal(t, 1, errors)
						} else {
							assert.Zero(t, errors)
						}
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
}
