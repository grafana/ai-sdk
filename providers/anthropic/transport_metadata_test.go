package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_TransportMetadata(t *testing.T) {
	const responseBody = `{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"Hello"}],"model":"claude-test","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`
	for _, vertex := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			name := "direct"
			if vertex {
				name = "vertex"
			}
			if streaming {
				name += "/stream"
			} else {
				name += "/generate"
			}
			t.Run(name, func(t *testing.T) {
				requests := make(chan []byte, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("reading outbound body: %v", err)
					}
					requests <- body
					w.Header().Set("X-Response-Proof", "actual-header")
					if streaming {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-test\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
						_, _ = io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n\n")
						_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, responseBody)
				}))
				defer server.Close()

				m := newTransportTestModel(t, server, vertex)
				maxTokens := 64
				params := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: &maxTokens}
				var returnedBody []byte
				var returnedHeaders map[string]string
				if streaming {
					result, err := m.DoStream(context.Background(), params)
					require.NoError(t, err)
					require.NotNil(t, result.Request)
					require.NotNil(t, result.Response)
					returnedBody = result.Request.Body
					returnedHeaders = result.Response.Headers
					for range result.Stream {
					}
				} else {
					result, err := m.DoGenerate(context.Background(), params)
					require.NoError(t, err)
					require.NotNil(t, result.Request)
					require.NotNil(t, result.Response)
					returnedBody = result.Request.Body
					returnedHeaders = result.Response.Headers
					assert.JSONEq(t, responseBody, string(result.Response.Body))
				}
				assert.Equal(t, "actual-header", returnedHeaders["x-response-proof"])
				select {
				case transmitted := <-requests:
					assert.Equal(t, transmitted, returnedBody)
					assert.JSONEq(t, string(transmitted), string(returnedBody))
					if vertex {
						assert.Contains(t, string(transmitted), `"anthropic_version":"vertex-2023-10-16"`)
						assert.NotContains(t, string(transmitted), `"model":"claude-test"`)
					} else {
						assert.Contains(t, string(transmitted), `"model":"claude-test"`)
					}
				default:
					t.Fatal("request not received")
				}
			})
		}
	}
}
