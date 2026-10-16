package openaicompatible

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutput_NamePresence(t *testing.T) {
	for _, name := range []*string{nil, new(""), new("answer")} {
		for _, streaming := range []bool{false, true} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Format struct {
						Schema struct {
							Name   *string         `json:"name"`
							Schema json.RawMessage `json:"schema"`
						} `json:"json_schema"`
					} `json:"response_format"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				expected := "response"
				if name != nil {
					expected = *name
				}
				require.NotNil(t, body.Format.Schema.Name)
				assert.Equal(t, expected, *body.Format.Schema.Name)
				assert.Contains(t, string(body.Format.Schema.Schema), "9007199254740993")
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: [DONE]\n\n"))
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{}"},"finish_reason":"stop"}]}`))
				}
			}))
			model := New("model", WithBaseURL(server.URL), WithStructuredOutputs(true))
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("json")}, ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Name: name, Schema: json.RawMessage(`{"type":"object","x-number":9007199254740993}`)}}
			if streaming {
				result, err := model.DoStream(context.Background(), opts)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err := model.DoGenerate(context.Background(), opts)
				require.NoError(t, err)
			}
			server.Close()
		}
	}
}
