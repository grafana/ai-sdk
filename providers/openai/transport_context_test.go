package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const transportResponseJSON = `{"id":"resp_transport","object":"response","created_at":1700000000,"model":"gpt-4o","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2},"unknown":{"retained":true}}`

var transportEvents = []string{
	`{"type":"response.created","response":{"id":"resp_transport","created_at":1700000000,"model":"gpt-4o","output":[]}}`,
	`{"type":"response.output_text.delta","item_id":"msg_1","delta":"hello"}`,
	`{"type":"response.completed","response":{"id":"resp_transport","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`,
}

func transportSSE(events []string) string {
	var out strings.Builder
	for _, event := range events {
		fmt.Fprintf(&out, "data: %s\n\n", event)
	}
	return out.String()
}

func transportResponse(r *http.Request, streaming bool) *http.Response {
	payload, contentType := transportResponseJSON, "application/json"
	if streaming {
		payload, contentType = transportSSE(transportEvents), "text/event-stream"
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}, "X-Transport": {"first", "second"}}, Body: io.NopCloser(strings.NewReader(payload)), Request: r}
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

func TestModel_TransportContext(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			var captured json.RawMessage
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				var err error
				captured, err = io.ReadAll(r.Body)
				assert.NoError(t, err)
				return transportResponse(r, streaming), nil
			})}
			m := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0), option.WithJSONSet("capture_marker", "body-option")))
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}}
			var request *provider.RequestMetadata
			var headers map[string]string
			if streaming {
				result, err := m.DoStream(t.Context(), opts)
				require.NoError(t, err)
				request = result.Request
				require.NotNil(t, result.Response)
				headers = result.Response.Headers
				var metadata int
				for part := range result.Stream {
					if part.Type == provider.PartResponseMeta {
						metadata++
						assert.Equal(t, headers, part.ResponseHeaders)
					}
				}
				assert.Equal(t, 1, metadata)
			} else {
				result, err := m.DoGenerate(t.Context(), opts)
				require.NoError(t, err)
				request, headers = result.Request, result.Response.Headers
				assert.JSONEq(t, transportResponseJSON, string(result.Response.Body))
			}
			require.NotNil(t, request)
			assert.JSONEq(t, string(captured), string(request.Body))
			assert.Equal(t, "first, second", headers["X-Transport"])
		})
	}
}
