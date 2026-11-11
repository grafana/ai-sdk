package bedrock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConverseParity_ReasoningOverridesRequestCapture(t *testing.T) {
	for _, namespace := range []string{"amazonBedrock", "bedrock"} {
		for _, streaming := range []bool{false, true} {
			for _, tc := range []struct {
				name    string
				modelID string
				config  string
				want    string
			}{
				{"disabled", "anthropic.claude-sonnet-4-6-v1:0", `{"type":"disabled","budgetTokens":5000,"maxReasoningEffort":"low"}`, ""},
				{"adaptive", "anthropic.claude-sonnet-4-6-v1:0", `{"maxReasoningEffort":"low"}`, `{"thinking":{"type":"adaptive"},"output_config":{"effort":"low"}}`},
				{"zero budget", testAnthropicModel, `{"budgetTokens":0}`, `{"thinking":{"type":"enabled","budget_tokens":0}}`},
				{"Nova", "us.amazon.nova-2-lite-v1:0", `{"maxReasoningEffort":"low"}`, `{"reasoningConfig":{"type":"enabled","maxReasoningEffort":"low"}}`},
				{"OpenAI OSS", testOpenAIModel, `{"maxReasoningEffort":"low"}`, `{"reasoning_effort":"low"}`},
				{"OpenAI", "openai.gpt-5", `{"maxReasoningEffort":"low"}`, `{"reasoning":{"effort":"low"}}`},
			} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", namespace, streaming, tc.name), func(t *testing.T) {
					requests := make(chan map[string]json.RawMessage, 1)
					streamBody := encodeFixtures(t, `{"messageStart":{"role":"assistant"}}`, `{"messageStop":{"stopReason":"end_turn"}}`, `{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var body map[string]json.RawMessage
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						requests <- body
						if streaming {
							assert.Contains(t, r.URL.Path, "/converse-stream")
							w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
							_, err := w.Write(streamBody)
							assert.NoError(t, err)
						} else {
							assert.Contains(t, r.URL.Path, "/converse")
							w.Header().Set("Content-Type", "application/json")
							_, err := io.WriteString(w, `{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`)
							assert.NoError(t, err)
						}
					}))
					defer server.Close()
					model := New(tc.modelID, WithBaseURL(server.URL), WithBearerToken("test-token"))
					maxTokens := 1024
					opts := provider.CallOptions{
						Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: provider.ReasoningXHigh, MaxOutputTokens: &maxTokens,
						ProviderOptions: provider.ProviderOptions{namespace: provider.RawProviderOption{Key: namespace, Raw: json.RawMessage(`{"reasoningConfig":` + tc.config + `}`)}},
					}
					if streaming {
						result, err := model.DoStream(context.Background(), opts)
						require.NoError(t, err)
						var starts, finishes int
						for part := range result.Stream {
							switch part.Type {
							case provider.PartStreamStart:
								starts++
								assert.Empty(t, part.Warnings)
							case provider.PartFinish:
								finishes++
							case provider.PartError:
								t.Errorf("unexpected stream error: %v", part.APICallError)
							}
						}
						assert.Equal(t, 1, starts)
						assert.Equal(t, 1, finishes)
					} else {
						result, err := model.DoGenerate(context.Background(), opts)
						require.NoError(t, err)
						assert.Empty(t, result.Warnings)
					}
					body := <-requests
					assert.JSONEq(t, `{"maxTokens":1024}`, string(body["inferenceConfig"]))
					if tc.want == "" {
						assert.NotContains(t, body, "additionalModelRequestFields")
					} else {
						assert.JSONEq(t, tc.want, string(body["additionalModelRequestFields"]))
					}
				})
			}
		}
	}
}
