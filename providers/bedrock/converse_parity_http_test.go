package bedrock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConverseParity_JSONToolRequestCapture(t *testing.T) {
	for _, tc := range []struct {
		name       string
		streaming  bool
		choice     provider.ToolChoice
		providerID string
	}{
		{name: "generate named choice", choice: provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "weather"}},
		{name: "stream none choice", streaming: true, choice: provider.ToolChoice{Type: provider.ToolChoiceNone}},
		{name: "generate provider tool", providerID: "anthropic.code_execution_20250522"},
		{name: "stream provider tool", streaming: true, providerID: "anthropic.code_execution_20250522"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ToolConfig struct {
						Tools []struct {
							ToolSpec struct {
								Name string `json:"name"`
							} `json:"toolSpec"`
						} `json:"tools"`
						ToolChoice json.RawMessage `json:"toolChoice"`
					} `json:"toolConfig"`
					AdditionalModelRequestFields map[string]json.RawMessage `json:"additionalModelRequestFields"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				var names []string
				for _, tool := range req.ToolConfig.Tools {
					names = append(names, tool.ToolSpec.Name)
				}
				if tc.providerID == "" {
					assert.Equal(t, []string{"weather", "search", "json"}, names)
					assert.JSONEq(t, `{"any":{}}`, string(req.ToolConfig.ToolChoice))
				} else {
					assert.Equal(t, []string{"code_execution", "json"}, names)
					assert.Empty(t, req.ToolConfig.ToolChoice)
					assert.JSONEq(t, `{"type":"any"}`, string(req.AdditionalModelRequestFields["tool_choice"]))
				}
				if tc.streaming {
					w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
					_, err := w.Write(encodeFixtures(t, `{"messageStart":{"role":"assistant"}}`, `{"messageStop":{"stopReason":"end_turn"}}`, `{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`))
					require.NoError(t, err)
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, err := w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}} ,"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
					require.NoError(t, err)
				}
			}))
			defer server.Close()
			model := New(testAnthropicModel, WithBaseURL(server.URL), WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")))
			opts := provider.CallOptions{
				Prompt:          []provider.Message{provider.UserText("hi")},
				ProviderOptions: provider.BuildProviderOptions(BedrockOptions{StructuredOutputMode: StructuredOutputModeJSONTool}),
				ResponseFormat:  &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`)},
			}
			if tc.providerID == "" {
				opts.ToolChoice = &tc.choice
				opts.Tools = []provider.Tool{{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)}, {Type: provider.ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{"type":"object"}`)}}
			} else {
				opts.Tools = []provider.Tool{{Type: provider.ToolTypeProvider, ID: tc.providerID, Name: "code_execution"}}
			}
			if tc.streaming {
				result, err := model.DoStream(context.Background(), opts)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err := model.DoGenerate(context.Background(), opts)
				require.NoError(t, err)
			}
		})
	}
}

func TestConverseParity_ProfileFamilyRequestCapture(t *testing.T) {
	const profile = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/opaque"
	for _, streaming := range []bool{false, true} {
		name := "generate"
		if streaming {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var req map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(body, &req))
				assert.JSONEq(t, `["/delta/stop_sequence"]`, string(req["additionalModelResponseFieldPaths"]))
				assert.Contains(t, string(req["additionalModelRequestFields"]), `"budget_tokens":2458`)
				if streaming {
					w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
					_, err = w.Write(encodeFixtures(t, `{"messageStart":{"role":"assistant"}}`, `{"messageStop":{"stopReason":"end_turn"}}`, `{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`))
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, err = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}} ,"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
				}
				require.NoError(t, err)
			}))
			defer server.Close()
			model := New(profile, WithModelFamily(ModelFamilyAnthropic), WithBaseURL(server.URL), WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")))
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: provider.ReasoningHigh}
			if streaming {
				result, err := model.DoStream(context.Background(), opts)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err := model.DoGenerate(context.Background(), opts)
				require.NoError(t, err)
			}
		})
	}
}
