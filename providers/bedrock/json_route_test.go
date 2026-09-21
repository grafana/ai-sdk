package bedrock

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

func TestModel_DefaultJSONToolRoute(t *testing.T) {
	for _, tc := range []struct {
		modelID  string
		fallback bool
	}{
		{"anthropic.claude-sonnet-4-6", true},
		{"us.anthropic.claude-haiku-4-5-20251001-v1:0", true},
		{testAnthropicModel, false},
		{"anthropic.claude-opus-4-6-v1", false},
	} {
		for _, thinking := range []bool{false, true} {
			for _, streaming := range []bool{false, true} {
				t.Run(tc.modelID+map[bool]string{false: "/plain", true: "/thinking"}[thinking]+map[bool]string{false: "/generate", true: "/stream"}[streaming], func(t *testing.T) {
					var request map[string]any
					frames := []string{`{"messageStart":{"role":"assistant"}}`}
					response := `{"text":"{\"foo\":\"bar\"}"}`
					stop := "end_turn"
					if tc.fallback {
						response = `{"toolUse":{"toolUseId":"call-1","name":"json","input":{"foo":"bar"}}}`
						stop = "tool_use"
						frames = append(frames,
							`{"contentBlockStart":{"contentBlockIndex":0,"start":{"toolUse":{"toolUseId":"call-1","name":"json"}}}}`,
							`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"toolUse":{"input":"{\"foo\":\"bar\"}"}}}}`,
						)
					} else {
						frames = append(frames, `{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"{\"foo\":\"bar\"}"}}}`)
					}
					frames = append(frames, `{"contentBlockStop":{"contentBlockIndex":0}}`, `{"messageStop":{"stopReason":"`+stop+`"}}`, `{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`)
					streamBody := encodeFixtures(t, frames...)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
						if streaming {
							w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
							_, err := w.Write(streamBody)
							assert.NoError(t, err)
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, err := w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[` + response + `]}},"stopReason":"` + stop + `","usage":{"inputTokens":1,"outputTokens":1}}`))
							assert.NoError(t, err)
						}
					}))
					defer server.Close()
					model := New(tc.modelID, WithRegion("us-east-1"), WithBaseURL(server.URL), WithBearerToken("test"))
					strict := true
					opts := provider.CallOptions{
						Prompt:         []provider.Message{provider.UserText("Hi")},
						ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object","properties":{"foo":{"type":"string"}},"additionalProperties":false}`)},
						Tools:          []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", Strict: &strict, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}},
					}
					if thinking {
						opts.Reasoning = provider.ReasoningHigh
					}
					var metadata provider.ProviderMetadata
					if streaming {
						result, err := model.DoStream(context.Background(), opts)
						require.NoError(t, err)
						var text string
						finished := false
						for part := range result.Stream {
							assert.NotEqual(t, provider.PartError, part.Type)
							assert.NotEqual(t, provider.PartToolCall, part.Type)
							assert.NotEqual(t, provider.PartToolInputStart, part.Type)
							if part.Type == provider.PartTextDelta {
								text += part.Delta
							}
							if part.Type == provider.PartFinish {
								finished = true
								assert.Equal(t, provider.FinishReasonStop, part.FinishReason.Unified)
								metadata = part.ProviderMetadata
							}
						}
						assert.True(t, finished)
						assert.JSONEq(t, `{"foo":"bar"}`, text)
					} else {
						result, err := model.DoGenerate(context.Background(), opts)
						require.NoError(t, err)
						require.Len(t, result.Content, 1)
						assert.Equal(t, provider.ContentText, result.Content[0].Type)
						assert.JSONEq(t, `{"foo":"bar"}`, result.Content[0].Text)
						assert.Equal(t, provider.FinishReasonStop, result.FinishReason.Unified)
						metadata = result.ProviderMetadata
					}
					var extras map[string]any
					if tc.fallback || len(metadata["amazonBedrock"]) > 0 {
						require.NoError(t, json.Unmarshal(metadata["amazonBedrock"], &extras))
					}
					toolConfig := request["toolConfig"].(map[string]any)
					tools := toolConfig["tools"].([]any)
					assert.Equal(t, true, tools[0].(map[string]any)["toolSpec"].(map[string]any)["strict"])
					if tc.fallback {
						assert.Equal(t, true, extras["isJsonResponseFromTool"])
						require.Len(t, tools, 2)
						assert.Equal(t, "json", tools[1].(map[string]any)["toolSpec"].(map[string]any)["name"])
						assert.Equal(t, map[string]any{"any": map[string]any{}}, toolConfig["toolChoice"])
						if fields, ok := request["additionalModelRequestFields"].(map[string]any); ok {
							if output, ok := fields["output_config"].(map[string]any); ok {
								assert.NotContains(t, output, "format")
							}
						}
					} else {
						assert.NotEqual(t, true, extras["isJsonResponseFromTool"])
						require.Len(t, tools, 1)
						fields := request["additionalModelRequestFields"].(map[string]any)
						assert.Contains(t, fields["output_config"], "format")
					}
				})
			}
		}
	}
}
