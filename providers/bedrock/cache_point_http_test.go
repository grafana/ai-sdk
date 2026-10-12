package bedrock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachePoint_SyntheticFilteredHistoryHTTP(t *testing.T) {
	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			requests := make(chan json.RawMessage, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages json.RawMessage `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				requests <- body.Messages
				if mode == "stream" {
					w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
					_, _ = w.Write(encodeFixtures(t, `{"messageStart":{"role":"assistant"}}`, `{"messageStop":{"stopReason":"end_turn"}}`, `{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`))
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
				}
			}))
			defer server.Close()
			model := New(testAnthropicModel, WithBaseURL(server.URL), WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")))
			opts := provider.CallOptions{Prompt: []provider.Message{
				provider.UserText("question"),
				cachedMessage(provider.RoleAssistant, provider.ToolCallPart("c1", "lookup", json.RawMessage(`{}`))),
				cachedMessage(provider.RoleTool, provider.ToolResultPart("c1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"})),
				cachedMessage(provider.RoleUser, provider.TextPart("next")),
			}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if mode == "stream" {
				result, err := model.DoStream(ctx, opts)
				require.NoError(t, err)
				var warnings []provider.Warning
				for part := range result.Stream {
					warnings = append(warnings, part.Warnings...)
				}
				assert.Contains(t, warningFeatures(warnings), "toolContent")
			} else {
				result, err := model.DoGenerate(ctx, opts)
				require.NoError(t, err)
				assert.Contains(t, warningFeatures(result.Warnings), "toolContent")
			}
			select {
			case body := <-requests:
				assert.JSONEq(t, `[{"role":"user","content":[{"text":"question"},{"text":"next"},{"cachePoint":{"type":"default","ttl":"5m"}}]}]`, string(body))
			case <-ctx.Done():
				t.Fatal("native request was not captured")
			}
		})
	}
}
