package bedrock

import (
	"context"
	"encoding/json"
	"fmt"
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
	const openObject = `{"type":"object","additionalProperties":true}`
	const toolSchema = `{"type":"object","additionalProperties":false}`
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct {
			name                string
			choice              provider.ToolChoice
			withProviderTool    bool
			want                []string
			wantAnthropicChoice bool
		}{
			{name: "none", choice: provider.ToolChoice{Type: provider.ToolChoiceNone}, withProviderTool: true, want: []string{"json"}},
			{name: "named", choice: provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}, withProviderTool: true, want: []string{"lookup", "json"}},
			{name: "named provider", choice: provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "execute"}, withProviderTool: true, want: []string{"execute", "json"}, wantAnthropicChoice: true},
			{name: "auto", choice: provider.ToolChoice{Type: provider.ToolChoiceAuto}, want: []string{"lookup", "unused", "json"}},
			{name: "required", choice: provider.ToolChoice{Type: provider.ToolChoiceRequired}, want: []string{"lookup", "unused", "json"}},
			{name: "auto with provider", choice: provider.ToolChoice{Type: provider.ToolChoiceAuto}, withProviderTool: true, want: []string{"execute", "lookup", "unused", "json"}, wantAnthropicChoice: true},
		} {
			t.Run(fmt.Sprintf("stream=%t/%s", streaming, tc.name), func(t *testing.T) {
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
					assert.Equal(t, tc.want, names)
					assert.NotContains(t, req.AdditionalModelRequestFields, "output_config")
					if tc.wantAnthropicChoice {
						assert.Empty(t, req.ToolConfig.ToolChoice)
						assert.JSONEq(t, `{"type":"any"}`, string(req.AdditionalModelRequestFields["tool_choice"]))
					} else {
						assert.JSONEq(t, `{"any":{}}`, string(req.ToolConfig.ToolChoice))
						assert.NotContains(t, req.AdditionalModelRequestFields, "tool_choice")
					}
					if streaming {
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
				model := New("anthropic.claude-opus-5", WithBaseURL(server.URL), WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")))
				opts := provider.CallOptions{
					Prompt:         []provider.Message{provider.UserText("hi")},
					ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(openObject)},
					ToolChoice:     &tc.choice,
					Tools: []provider.Tool{
						{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(toolSchema)},
						{Type: provider.ToolTypeFunction, Name: "unused", InputSchema: json.RawMessage(toolSchema)},
					},
				}
				if tc.withProviderTool {
					opts.Tools = append(opts.Tools, provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250522", Name: "execute"})
				}
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

func TestConverseParity_NovaHighReasoningMaxTokensRequestCapture(t *testing.T) {
	const modelID = "us.amazon.nova-2-lite-v1:0"
	want := provider.Warning{
		Type:    provider.WarnUnsupported,
		Feature: "maxOutputTokens",
		Details: "maxOutputTokens is not supported by " + modelID + " when high reasoning is enabled and will be ignored",
	}
	for _, tc := range []struct {
		name      string
		streaming bool
		explicit  bool
	}{
		{name: "generate portable"},
		{name: "stream portable", streaming: true},
		{name: "generate explicit", explicit: true},
		{name: "stream explicit", streaming: true, explicit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req map[string]json.RawMessage
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				assert.NotContains(t, req, "inferenceConfig")
				assert.JSONEq(t, `{"reasoningConfig":{"type":"enabled","maxReasoningEffort":"high"}}`, string(req["additionalModelRequestFields"]))
				var err error
				if tc.streaming {
					w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
					_, err = w.Write(encodeFixtures(t, `{"messageStart":{"role":"assistant"}}`, `{"messageStop":{"stopReason":"end_turn"}}`, `{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`))
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, err = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}} ,"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
				}
				require.NoError(t, err)
			}))
			defer server.Close()
			model := New(modelID, WithBaseURL(server.URL), WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")))
			maxTokens := 1024
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: provider.ReasoningHigh, MaxOutputTokens: &maxTokens}
			if tc.explicit {
				opts.Reasoning = provider.ReasoningProviderDefault
				opts.ProviderOptions = provider.BuildProviderOptions(BedrockOptions{ReasoningConfig: &ReasoningConfig{Type: "enabled", MaxReasoningEffort: "high"}})
			}
			var warnings []provider.Warning
			if tc.streaming {
				result, err := model.DoStream(context.Background(), opts)
				require.NoError(t, err)
				for part := range result.Stream {
					if part.Type == provider.PartStreamStart {
						warnings = part.Warnings
					}
				}
			} else {
				result, err := model.DoGenerate(context.Background(), opts)
				require.NoError(t, err)
				warnings = result.Warnings
			}
			assert.Contains(t, warnings, want)
		})
	}
}
