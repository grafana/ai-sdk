package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collectHTTPStream(t *testing.T, events []string, opts provider.CallOptions) []provider.StreamPart {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range events {
			_, err := fmt.Fprintf(w, "data: %s\n\n", data)
			assert.NoError(t, err)
		}
	}))
	defer server.Close()
	model := NewResponses("test", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL), option.WithMaxRetries(0)))
	result, err := model.DoStream(context.Background(), opts)
	require.NoError(t, err)
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	assert.Equal(t, int32(1), requests.Load())
	return parts
}

func TestDoStream_RawTransportEvents(t *testing.T) {
	for _, tc := range []struct {
		name       string
		events     []string
		raw        []string
		errorFrame bool
	}{
		{name: "normal", events: transportEvents, raw: transportEvents},
		{name: "ignored", events: []string{transportEvents[0], `{"type":"future.event","extra":42}`, transportEvents[2]}, raw: []string{transportEvents[0], `{"type":"future.event","extra":42}`, transportEvents[2]}},
		{name: "invalid JSON", events: []string{transportEvents[1], `not JSON`}, raw: []string{transportEvents[1], ""}, errorFrame: true},
		{name: "typed failure", events: []string{transportEvents[1], `"not an event"`}, raw: []string{transportEvents[1], `"not an event"`}, errorFrame: true},
		{name: "null", events: []string{transportEvents[1], `null`}, raw: []string{transportEvents[1], `null`}},
		{name: "error", events: []string{transportEvents[1], `{"error":{"message":"failed","type":"server_error"}}`}, raw: []string{transportEvents[1], `{"error":{"message":"failed","type":"server_error"}}`}, errorFrame: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var normalizedJSON []byte
			for _, includeRaw := range []bool{false, true} {
				m := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithMaxRetries(0), option.WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
						if want := map[provider.StreamPartType]string{provider.PartResponseMeta: transportEvents[0], provider.PartTextStart: transportEvents[1], provider.PartTextDelta: transportEvents[1], provider.PartTextEnd: transportEvents[2], provider.PartFinish: transportEvents[2]}[part.Type]; want != "" {
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

func TestDoStream_RecoversMalformedEvents(t *testing.T) {
	for _, tc := range []struct {
		name      string
		terminal  string
		lateError bool
	}{
		{name: "completed", terminal: `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`},
		{name: "incomplete", terminal: `{"type":"response.incomplete","response":{"id":"resp_1","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`},
		{name: "malformed after completion", terminal: `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`, lateError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := []string{
				`{"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
				`{"type":"response.output_text.delta","output_index":0,"item_id":"msg_1","delta":"hello"}`,
				`{"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"hello","annotations":[]}]}}`,
				tc.terminal,
			}
			if tc.lateError {
				events = append(events, `not JSON`)
			} else {
				events = append([]string{`{"type":"response.created"`, `not JSON`}, events...)
			}
			events = append(events, `[DONE]`)
			parts := collectHTTPStream(t, events, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
			var errors, finishes int
			var text string
			for _, part := range parts {
				switch part.Type {
				case provider.PartError:
					errors++
					require.NotNil(t, part.APICallError)
					assert.False(t, part.APICallError.IsRetryable)
				case provider.PartTextDelta:
					text += part.Delta
				case provider.PartFinish:
					finishes++
					require.NotNil(t, part.FinishReason)
					assert.Equal(t, provider.FinishReason{Unified: provider.FinishReasonError}, *part.FinishReason)
					require.NotNil(t, part.Usage)
					require.NotNil(t, part.Usage.InputTokens.Total)
					require.NotNil(t, part.Usage.OutputTokens.Total)
					assert.Equal(t, 3, *part.Usage.InputTokens.Total)
					assert.Equal(t, 2, *part.Usage.OutputTokens.Total)
				}
			}
			if tc.lateError {
				assert.Equal(t, 1, errors)
			} else {
				assert.Equal(t, 2, errors)
			}
			assert.Equal(t, "hello", text)
			assert.Equal(t, 1, finishes)
			require.NotEmpty(t, parts)
			assert.Equal(t, provider.PartFinish, parts[len(parts)-1].Type)
		})
	}
}

func TestDoStream_TruncatedParallelInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		last string
	}{
		{name: "EOF"},
		{name: "malformed done", last: `{"type":"response.output_item.done"`},
		{name: "completed without item done", last: `{"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := []string{
				`{"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","id":"fc_b","call_id":"call_b","name":"parallel","arguments":""}}`,
				`{"type":"response.function_call_arguments.delta","output_index":2,"item_id":"fc_b","delta":"second"}`,
				`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_a","call_id":"call_a","name":"parallel","arguments":""}}`,
				`{"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_a","delta":"first"}`,
			}
			if tc.last != "" {
				events = append(events, tc.last)
			}
			parts := collectHTTPStream(t, events, provider.CallOptions{})
			var toolParts []provider.StreamPart
			finished := false
			for _, part := range parts {
				switch part.Type {
				case provider.PartFinish:
					finished = true
				case provider.PartToolInputStart, provider.PartToolInputDelta, provider.PartToolInputEnd, provider.PartToolCall:
					assert.False(t, finished, "tool input must precede finish")
					toolParts = append(toolParts, part)
				}
			}
			assert.Equal(t, []provider.StreamPart{
				{Type: provider.PartToolInputStart, ID: "call_a", ToolName: "parallel"},
				{Type: provider.PartToolInputDelta, ID: "call_a", Delta: "first"},
				{Type: provider.PartToolInputStart, ID: "call_b", ToolName: "parallel"},
				{Type: provider.PartToolInputDelta, ID: "call_b", Delta: "second"},
			}, toolParts)
		})
	}
}
