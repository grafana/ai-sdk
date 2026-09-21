package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testAnthropicToolOption struct {
	DisableParallelToolUse *bool `json:"disableParallelToolUse,omitempty"`
}

func (testAnthropicToolOption) ProviderKey() string { return "anthropic" }

func TestBuildRequest_InvalidParallelToolOption(t *testing.T) {
	for _, value := range []string{`null`, `"true"`, `{}`} {
		t.Run(value, func(t *testing.T) {
			_, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"disableParallelToolUse":` + value + `}`)}}})
			require.ErrorContains(t, err, "anthropic provider options")
		})
	}
}

func TestModel_DisableParallelToolUse(t *testing.T) {
	trueFlag, falseFlag := true, false
	functionTools := []provider.Tool{
		{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Type: provider.ToolTypeFunction, Name: "other", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	providerTool := provider.Tool{Type: provider.ToolTypeProvider, Name: "terminal", ID: "anthropic.bash_20250124"}
	mixedTools := append(append([]provider.Tool{}, functionTools...), providerTool)
	for _, tc := range []struct {
		name      string
		modelID   string
		flag      *bool
		choice    provider.ToolChoiceType
		tools     []provider.Tool
		json      bool
		want      string
		converse  string
		toolCount int
		warnings  int
	}{
		{name: "default", flag: &trueFlag, tools: functionTools, want: `{"type":"auto","disable_parallel_tool_use":true}`, toolCount: 2},
		{name: "auto", flag: &trueFlag, choice: provider.ToolChoiceAuto, tools: functionTools, want: `{"type":"auto","disable_parallel_tool_use":true}`, toolCount: 2},
		{name: "required", flag: &trueFlag, choice: provider.ToolChoiceRequired, tools: functionTools, want: `{"type":"any","disable_parallel_tool_use":true}`, toolCount: 2},
		{name: "named", flag: &trueFlag, choice: provider.ToolChoiceTool, tools: functionTools, want: `{"type":"tool","name":"lookup","disable_parallel_tool_use":true}`, toolCount: 1},
		{name: "none", flag: &trueFlag, choice: provider.ToolChoiceNone, tools: functionTools},
		{name: "no tools", flag: &trueFlag, choice: provider.ToolChoiceRequired},
		{name: "false", flag: &falseFlag, choice: provider.ToolChoiceRequired, tools: functionTools, converse: `{"any":{}}`, toolCount: 2},
		{name: "unset", choice: provider.ToolChoiceAuto, tools: functionTools, converse: `{"auto":{}}`, toolCount: 2},
		{name: "non anthropic", modelID: "openai.gpt-oss-120b-1:0", flag: &trueFlag, choice: provider.ToolChoiceRequired, tools: functionTools, converse: `{"any":{}}`, toolCount: 2},
		{name: "profile", modelID: testApplicationProfile, flag: &trueFlag, tools: functionTools, want: `{"type":"auto","disable_parallel_tool_use":true}`, toolCount: 2},
		{name: "provider default", flag: &trueFlag, tools: mixedTools, want: `{"type":"auto","disable_parallel_tool_use":true}`, toolCount: 3},
		{name: "provider required", flag: &trueFlag, choice: provider.ToolChoiceRequired, tools: mixedTools, want: `{"type":"any","disable_parallel_tool_use":true}`, toolCount: 3},
		{name: "provider false", flag: &falseFlag, tools: mixedTools, toolCount: 3},
		{name: "provider explicit auto false", flag: &falseFlag, choice: provider.ToolChoiceAuto, tools: mixedTools, want: `{"type":"auto","disable_parallel_tool_use":false}`, toolCount: 3},
		{name: "provider unset", tools: mixedTools, toolCount: 3},
		{name: "provider none", flag: &trueFlag, choice: provider.ToolChoiceNone, tools: mixedTools, toolCount: 3},
		{name: "json only", modelID: "anthropic.claude-sonnet-4-6", flag: &trueFlag, json: true, want: `{"type":"any","disable_parallel_tool_use":true}`, toolCount: 1},
		{name: "json overrides none", modelID: "anthropic.claude-sonnet-4-6", flag: &trueFlag, json: true, choice: provider.ToolChoiceNone, want: `{"type":"any","disable_parallel_tool_use":true}`, toolCount: 1},
		{name: "json mixed preserves declarations", modelID: "anthropic.claude-sonnet-4-6", flag: &trueFlag, json: true, choice: provider.ToolChoiceTool, tools: mixedTools, want: `{"type":"any","disable_parallel_tool_use":true}`, toolCount: 4},
		{name: "json false", modelID: "anthropic.claude-sonnet-4-6", flag: &falseFlag, json: true, converse: `{"any":{}}`, toolCount: 1},
		{name: "filtered web", flag: &trueFlag, tools: []provider.Tool{{Type: provider.ToolTypeProvider, Name: "web", ID: unsupportedWebSearchToolID}}, warnings: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, rawOption := range []bool{false, true} {
				for _, streaming := range []bool{false, true} {
					t.Run(map[bool]string{false: "typed", true: "raw"}[rawOption]+map[bool]string{false: "/generate", true: "/stream"}[streaming], func(t *testing.T) {
						modelID := tc.modelID
						if modelID == "" {
							modelID = testAnthropicModel
						}
						anthropicOption := testAnthropicToolOption{DisableParallelToolUse: tc.flag}
						opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("Hi")}, Tools: tc.tools, ProviderOptions: provider.BuildProviderOptions(anthropicOption)}
						if rawOption {
							raw, err := json.Marshal(anthropicOption)
							require.NoError(t, err)
							opts.ProviderOptions["anthropic"] = provider.RawProviderOption{Key: "anthropic", Raw: raw}
						}
						if modelID == testApplicationProfile {
							opts.ProviderOptions["amazonBedrock"] = BedrockOptions{ReasoningConfig: &ReasoningConfig{Type: "enabled", BudgetTokens: 1024}}
						}
						if tc.choice != "" {
							opts.ToolChoice = &provider.ToolChoice{Type: tc.choice}
							if tc.choice == provider.ToolChoiceTool {
								opts.ToolChoice.ToolName = "lookup"
							}
						}
						if tc.json {
							opts.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object"}`)}
						}
						before, err := json.Marshal(opts)
						require.NoError(t, err)
						body, warnings := captureBedrockRequest(t, modelID, opts, streaming)
						assert.Len(t, warnings, tc.warnings)
						fields, _ := body["additionalModelRequestFields"].(map[string]any)
						if tc.want == "" {
							assert.NotContains(t, fields, "tool_choice")
						} else {
							raw, err := json.Marshal(fields["tool_choice"])
							require.NoError(t, err)
							assert.JSONEq(t, tc.want, string(raw))
						}
						if tc.toolCount == 0 {
							assert.NotContains(t, body, "toolConfig")
						} else {
							toolConfig, ok := body["toolConfig"].(map[string]any)
							require.True(t, ok)
							assert.Len(t, toolConfig["tools"], tc.toolCount)
							if tc.converse == "" {
								assert.NotContains(t, toolConfig, "toolChoice")
							} else {
								raw, err := json.Marshal(toolConfig["toolChoice"])
								require.NoError(t, err)
								assert.JSONEq(t, tc.converse, string(raw))
							}
						}
						after, err := json.Marshal(opts)
						require.NoError(t, err)
						assert.JSONEq(t, string(before), string(after))
					})
				}
			}
		})
	}
}
