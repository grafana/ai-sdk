package bedrock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareTools_UnsupportedProviderWarnings(t *testing.T) {
	unknown := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.unknown_tool", Name: "unknown"}
	other := provider.Tool{Type: provider.ToolTypeProvider, ID: "custom.other_tool", Name: "other"}
	web := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.web_search_20250305", Name: "web"}
	known := provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250522", Name: "code_execution"}
	function := provider.Tool{Type: provider.ToolTypeFunction, Name: "weather"}
	sharedUnknown := provider.Warning{Type: provider.WarnUnsupported, Feature: "provider-defined tool anthropic.unknown_tool"}
	sharedOther := provider.Warning{Type: provider.WarnUnsupported, Feature: "provider-defined tool custom.other_tool"}
	bedrockUnknown := provider.Warning{Type: provider.WarnUnsupported, Feature: "tool anthropic.unknown_tool"}
	bedrockOther := provider.Warning{Type: provider.WarnUnsupported, Feature: "tool custom.other_tool"}
	webWarning := provider.Warning{Type: provider.WarnUnsupported, Feature: "web_search_20250305 tool", Details: "The web_search_20250305 tool is not supported on Amazon Bedrock."}
	for _, tc := range []struct {
		name      string
		tools     []provider.Tool
		modelID   string
		anthropic bool
		choice    *provider.ToolChoice
		warnings  []provider.Warning
		names     []string
	}{
		{name: "Anthropic unknown", tools: []provider.Tool{unknown}, modelID: testAnthropicModel, anthropic: true, warnings: []provider.Warning{sharedUnknown, bedrockUnknown}},
		{name: "Anthropic known and unknown", tools: []provider.Tool{unknown, known, function}, modelID: testAnthropicModel, anthropic: true, warnings: []provider.Warning{sharedUnknown, bedrockUnknown}, names: []string{"code_execution", "weather"}},
		{name: "Anthropic none choice", tools: []provider.Tool{unknown}, modelID: testAnthropicModel, anthropic: true, choice: &provider.ToolChoice{Type: provider.ToolChoiceNone}, warnings: []provider.Warning{sharedUnknown, bedrockUnknown}},
		{name: "Nova unknown", tools: []provider.Tool{unknown}, modelID: "amazon.nova-pro-v1:0", warnings: []provider.Warning{bedrockUnknown}},
		{name: "Anthropic grouped warnings", tools: []provider.Tool{unknown, function, other, web}, modelID: testAnthropicModel, anthropic: true, warnings: []provider.Warning{webWarning, sharedUnknown, sharedOther, bedrockUnknown, bedrockOther}, names: []string{"weather"}},
		{name: "Nova grouped warnings", tools: []provider.Tool{unknown, function, other, web}, modelID: "amazon.nova-pro-v1:0", warnings: []provider.Warning{webWarning, bedrockUnknown, bedrockOther}, names: []string{"weather"}},
		{name: "known web tool", tools: []provider.Tool{web}, modelID: testAnthropicModel, anthropic: true, warnings: []provider.Warning{webWarning}},
		{name: "required unknown", tools: []provider.Tool{unknown, other}, modelID: testSonnet55Model, anthropic: true, choice: &provider.ToolChoice{Type: provider.ToolChoiceRequired}, warnings: []provider.Warning{sharedUnknown, sharedOther, {Type: provider.WarnUnsupported, Feature: "toolChoice", Details: "toolChoice 'required' is not supported with this model or thinking configuration. Using 'auto' instead. Instruct the model to use a tool in the prompt and verify that a tool call was made."}, bedrockUnknown, bedrockOther}},
		{name: "named unknown", tools: []provider.Tool{unknown, other, function}, modelID: testSonnet55Model, anthropic: true, choice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "unknown"}, warnings: []provider.Warning{sharedUnknown, sharedOther, {Type: provider.WarnUnsupported, Feature: "toolChoice", Details: "toolChoice 'tool' is not supported with this model or thinking configuration. Only the 'unknown' tool is sent with 'auto' tool choice. Instruct the model to use the tool in the prompt and verify that a tool call was made."}, bedrockUnknown}},
		{name: "named function", tools: []provider.Tool{unknown, function}, modelID: testSonnet55Model, anthropic: true, choice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "weather"}, warnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: "toolChoice", Details: "toolChoice 'tool' is not supported by this model because it rejects forced tool use. Only the 'weather' tool is sent with 'auto' tool choice. Instruct the model to use the tool in the prompt and verify that a tool call was made."}}, names: []string{"weather"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := prepareTools(tc.tools, tc.choice, tc.modelID, tc.anthropic, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.warnings, got.warnings)
			var names []string
			if got.toolConfig != nil {
				for _, tool := range got.toolConfig.Tools {
					names = append(names, tool.ToolSpec.Name)
				}
			}
			assert.Equal(t, tc.names, names)
		})
	}
}

func TestConverse_UnsupportedProviderWarnings(t *testing.T) {
	for _, modelID := range []string{testAnthropicModel, testNovaModel} {
		for _, withFunction := range []bool{false, true} {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/function=%t/stream=%t", modelID, withFunction, streaming), func(t *testing.T) {
					requests := make(chan map[string]json.RawMessage, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var request map[string]json.RawMessage
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						requests <- request
						if streaming {
							w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
							_, _ = w.Write(encodeFixtures(t, `{"messageStart":{"role":"assistant"}}`, `{"messageStop":{"stopReason":"end_turn"}}`, `{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`))
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
						}
					}))
					defer server.Close()
					model := New(modelID, WithBaseURL(server.URL), WithBearerToken("synthetic"))
					opts := provider.CallOptions{
						Prompt: []provider.Message{provider.UserText("hi")},
						Tools:  []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.unknown_tool", Name: "unknown"}},
					}
					if withFunction {
						opts.Tools = append(opts.Tools, provider.Tool{Type: provider.ToolTypeFunction, Name: "weather"})
					}
					var warnings []provider.Warning
					if streaming {
						result, err := model.DoStream(context.Background(), opts)
						require.NoError(t, err)
						for part := range result.Stream {
							assert.NotEqual(t, provider.PartError, part.Type)
							if part.Type == provider.PartStreamStart {
								warnings = part.Warnings
							}
						}
					} else {
						result, err := model.DoGenerate(context.Background(), opts)
						require.NoError(t, err)
						warnings = result.Warnings
					}
					var want []provider.Warning
					if modelID == testAnthropicModel {
						want = append(want, provider.Warning{Type: provider.WarnUnsupported, Feature: "provider-defined tool anthropic.unknown_tool"})
					}
					want = append(want, provider.Warning{Type: provider.WarnUnsupported, Feature: "tool anthropic.unknown_tool"})
					assert.Equal(t, want, warnings)
					request := <-requests
					assert.JSONEq(t, `[{"role":"user","content":[{"text":"hi"}]}]`, string(request["messages"]))
					assert.NotContains(t, request, "additionalModelRequestFields")
					if withFunction {
						assert.JSONEq(t, `{"tools":[{"toolSpec":{"name":"weather","inputSchema":{"json":{"type":"object","properties":{}}}}}]}`, string(request["toolConfig"]))
					} else {
						assert.NotContains(t, request, "toolConfig")
					}
				})
			}
		}
	}
}
