package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPresentEmptyReasoningNativeReplay(t *testing.T) {
	for _, tc := range []struct{ name, output, metadata, native string }{
		{"signature", `{"type":"thinking","thinking":"","signature":""}`, `{"signature":""}`, `{"type":"thinking","thinking":"","signature":""}`},
		{"redacted", `{"type":"redacted_thinking","data":""}`, `{"redactedData":""}`, `{"type":"redacted_thinking","data":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan map[string]json.RawMessage, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]json.RawMessage
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				requests <- body
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"r","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[`+tc.output+`],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
			}))
			defer server.Close()
			model := New("dummy", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
			limit := 64
			first, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("think")}, MaxOutputTokens: &limit})
			require.NoError(t, err)
			<-requests
			require.Len(t, first.Content, 1)
			assert.JSONEq(t, tc.metadata, string(first.Content[0].ProviderMetadata["anthropic"]))
			part := provider.ReasoningPart(first.Content[0].Text)
			part.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: first.Content[0].ProviderMetadata["anthropic"]}}
			_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(part, provider.TextPart("visible")), provider.UserText("continue")}, MaxOutputTokens: &limit})
			require.NoError(t, err)
			body := <-requests
			var messages []struct {
				Content []json.RawMessage `json:"content"`
			}
			require.NoError(t, json.Unmarshal(body["messages"], &messages))
			require.Len(t, messages[0].Content, 2)
			assert.JSONEq(t, tc.native, string(messages[0].Content[0]))
		})
	}
}
