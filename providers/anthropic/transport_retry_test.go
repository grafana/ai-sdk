package anthropic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_TransportMetadataAfterRetry(t *testing.T) {
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
				requests := make(chan []byte, 2)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("reading outbound body: %v", err)
					}
					requests <- body
					if len(requests) == 1 {
						w.Header().Set("Retry-After-Ms", "0")
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusTooManyRequests)
						_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"retry"}}`)
						return
					}
					w.Header().Set("X-Attempt", "second")
					if streaming {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-test\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-test","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
				}))
				defer server.Close()
				m := newTransportTestModel(t, server, vertex, option.WithMaxRetries(1))
				maxTokens := 64
				params := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: &maxTokens}
				var gotBody []byte
				var gotHeaders map[string]string
				if streaming {
					result, err := m.DoStream(context.Background(), params)
					require.NoError(t, err)
					require.NotNil(t, result.Request)
					gotBody, gotHeaders = result.Request.Body, result.Response.Headers
					for range result.Stream {
					}
				} else {
					result, err := m.DoGenerate(context.Background(), params)
					require.NoError(t, err)
					require.NotNil(t, result.Request)
					gotBody, gotHeaders = result.Request.Body, result.Response.Headers
				}
				require.Len(t, requests, 2)
				<-requests
				assert.Equal(t, <-requests, gotBody)
				assert.Equal(t, "second", gotHeaders["x-attempt"])
			})
		}
	}
}
