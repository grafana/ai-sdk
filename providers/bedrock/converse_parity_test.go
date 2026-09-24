package bedrock

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConverseParity_ModelFamilyAndReasoning(t *testing.T) {
	profile := "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/opaque"
	cases := []struct {
		name, modelID string
		family        ModelFamily
		budget        int
		reasoning     provider.ReasoningEffort
		thinking      string
		budgetOut     int
		responsePath  bool
	}{
		{name: "explicit family", modelID: profile, family: ModelFamilyAnthropic, reasoning: provider.ReasoningHigh, thinking: "enabled", budgetOut: 2458, responsePath: true},
		{name: "profile with explicit budget", modelID: profile, budget: 1024, thinking: "enabled", budgetOut: 1024, responsePath: true},
		{name: "opaque profile", modelID: profile, reasoning: provider.ReasoningProviderDefault},
		{name: "regional dated sonnet", modelID: "us.anthropic.claude-sonnet-4-5-20250929-v1:0", reasoning: provider.ReasoningHigh, thinking: "enabled", budgetOut: 38400, responsePath: true},
		{name: "opus 4.1", modelID: "anthropic.claude-opus-4-1-20250805-v1:0", reasoning: provider.ReasoningHigh, thinking: "enabled", budgetOut: 19200, responsePath: true},
		{name: "opus 4.5", modelID: "anthropic.claude-opus-4-5-20251101-v1:0", reasoning: provider.ReasoningHigh, thinking: "enabled", budgetOut: 38400, responsePath: true},
		{name: "sonnet 4.6", modelID: "anthropic.claude-sonnet-4-6-v1:0", reasoning: provider.ReasoningHigh, thinking: "adaptive", responsePath: true},
		{name: "unknown Claude", modelID: "anthropic.claude-future-v1:0", reasoning: provider.ReasoningHigh, thinking: "adaptive", responsePath: true},
		{name: "legacy Claude", modelID: "anthropic.claude-3-haiku-v1:0", reasoning: provider.ReasoningHigh, thinking: "enabled", budgetOut: 2458, responsePath: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bo := BedrockOptions{}
			if tc.budget > 0 {
				bo.ReasoningConfig = &ReasoningConfig{Type: "enabled", BudgetTokens: tc.budget}
			}
			req, warnings, _, err := buildRequestWithFamily(tc.modelID, tc.family, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: tc.reasoning,
				ProviderOptions: provider.BuildProviderOptions(bo),
			})
			require.NoError(t, err)
			if tc.responsePath {
				assert.Equal(t, []string{"/delta/stop_sequence"}, req.AdditionalModelResponseFieldPaths)
			} else {
				assert.Empty(t, req.AdditionalModelResponseFieldPaths)
			}
			if tc.thinking == "" {
				assert.NotContains(t, req.AdditionalModelRequestFields, "thinking")
			} else {
				thinking, ok := req.AdditionalModelRequestFields["thinking"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tc.thinking, thinking["type"])
				if tc.budgetOut > 0 {
					assert.Equal(t, tc.budgetOut, thinking["budget_tokens"])
				} else {
					assert.NotContains(t, thinking, "budget_tokens")
				}
			}
			if tc.family != "" {
				assert.Empty(t, warnings)
			}
		})
	}
}

func TestConverseParity_OutputMode(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`)
	cases := []struct {
		name, modelID             string
		mode                      StructuredOutputMode
		withTool                  bool
		native, tool, instruction bool
	}{
		{name: "sonnet 4.6 auto fallback", modelID: "anthropic.claude-sonnet-4-6", tool: true},
		{name: "haiku 4.5 auto fallback", modelID: "anthropic.claude-haiku-4-5", tool: true},
		{name: "older sonnet 4 auto fallback", modelID: "anthropic.claude-sonnet-4-20250514-v1:0", tool: true},
		{name: "older opus 4 auto fallback", modelID: "anthropic.claude-opus-4-20250514-v1:0", tool: true},
		{name: "older opus 4.1 native", modelID: "anthropic.claude-opus-4-1-20250805-v1:0", native: true},
		{name: "unknown claude native", modelID: "us.anthropic.claude-future-9-20990101-v1:0", native: true},
		{name: "legacy claude fallback", modelID: "anthropic.claude-3-haiku-20240307-v1:0", tool: true},
		{name: "non-anthropic fallback", modelID: "mistral.mistral-large-2407-v1:0", tool: true},
		{name: "sonnet 4.6 explicit format", modelID: "anthropic.claude-sonnet-4-6", mode: StructuredOutputModeOutputFormat, native: true},
		{name: "sonnet 4.5 json tool", modelID: testAnthropicModel, mode: StructuredOutputModeJSONTool, tool: true},
		{name: "opus 4.8 auto user tools", modelID: "anthropic.claude-opus-4-8", withTool: true, instruction: true},
		{name: "opus 4.8 forced tool with user tools", modelID: "anthropic.claude-opus-4-8", mode: StructuredOutputModeJSONTool, withTool: true, tool: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := map[string]any{"effort": "high"}
			if tc.mode == StructuredOutputModeJSONTool {
				config["format"] = "old"
			}
			bo := BedrockOptions{StructuredOutputMode: tc.mode, AdditionalModelRequestFields: map[string]any{"output_config": config}}
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema}, ProviderOptions: provider.BuildProviderOptions(bo)}
			if tc.withTool {
				opts.Tools = []provider.Tool{{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)}}
			}
			req, _, meta, err := buildRequestWithFamily(tc.modelID, "", opts)
			require.NoError(t, err)
			assert.Equal(t, tc.tool, meta.usesJSONResponseTool)
			assert.Equal(t, tc.instruction, meta.usesJSONInstruction)
			oc, _ := req.AdditionalModelRequestFields["output_config"].(map[string]any)
			if tc.native {
				assert.IsType(t, map[string]any{}, oc["format"])
			} else {
				assert.NotContains(t, oc, "format")
			}
			assert.Equal(t, "high", oc["effort"])
			if tc.mode == StructuredOutputModeJSONTool {
				assert.Equal(t, "old", config["format"])
			}
			if tc.tool {
				require.NotNil(t, req.ToolConfig)
				assert.Equal(t, "json", req.ToolConfig.Tools[len(req.ToolConfig.Tools)-1].ToolSpec.Name)
			}
		})
	}
}

func TestConverseParity_JSONToolKeepsCallerTools(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`)
	for _, choice := range []provider.ToolChoice{{Type: provider.ToolChoiceNone}, {Type: provider.ToolChoiceTool, ToolName: "weather"}} {
		t.Run(string(choice.Type), func(t *testing.T) {
			req, _, meta := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
				Prompt:          []provider.Message{provider.UserText("hi")},
				ProviderOptions: provider.BuildProviderOptions(BedrockOptions{StructuredOutputMode: StructuredOutputModeJSONTool}),
				ResponseFormat:  &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
				ToolChoice:      &choice,
				Tools: []provider.Tool{
					{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)},
					{Type: provider.ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{"type":"object"}`)},
				},
			})
			assert.True(t, meta.usesJSONResponseTool)
			require.NotNil(t, req.ToolConfig)
			require.Len(t, req.ToolConfig.Tools, 3)
			assert.Equal(t, []string{"weather", "search", "json"}, []string{req.ToolConfig.Tools[0].ToolSpec.Name, req.ToolConfig.Tools[1].ToolSpec.Name, req.ToolConfig.Tools[2].ToolSpec.Name})
			assert.Equal(t, &toolChoiceUnion{Any: &struct{}{}}, req.ToolConfig.ToolChoice)
		})
	}

	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hi")},
		ProviderOptions: provider.BuildProviderOptions(BedrockOptions{StructuredOutputMode: StructuredOutputModeJSONTool}),
		ResponseFormat:  &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
		Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250522", Name: "code_execution"}},
	})
	require.NotNil(t, req.ToolConfig)
	require.Len(t, req.ToolConfig.Tools, 2)
	assert.Nil(t, req.ToolConfig.ToolChoice)
	assert.Equal(t, map[string]any{"type": "any"}, req.AdditionalModelRequestFields["tool_choice"])
}

func TestConverseParity_ZeroBudgetIdentifiesProfile(t *testing.T) {
	profile := "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/opaque"
	req, warnings, _, err := buildRequestWithFamily(profile, "", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		ProviderOptions: provider.ProviderOptions{
			"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`{"reasoningConfig":{"type":"enabled","budgetTokens":0}}`)},
		},
		Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250522", Name: "code_execution"}},
	})
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, []string{"/delta/stop_sequence"}, req.AdditionalModelResponseFieldPaths)
	assert.Equal(t, map[string]any{"type": "enabled", "budget_tokens": 0}, req.AdditionalModelRequestFields["thinking"])
	require.NotNil(t, req.InferenceConfig)
	require.NotNil(t, req.InferenceConfig.MaxTokens)
	assert.Equal(t, 4096, *req.InferenceConfig.MaxTokens)
	require.NotNil(t, req.ToolConfig)
	require.Len(t, req.ToolConfig.Tools, 1)

	options := provider.ProviderOptions{
		"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`{"reasoningConfig":{"type":"enabled","budgetTokens":0}}`)},
	}
	req, warnings, _, err = buildRequestWithFamily(testAnthropicModel, "", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: provider.ReasoningHigh, ProviderOptions: options,
	})
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, map[string]any{"type": "enabled", "budget_tokens": 0}, req.AdditionalModelRequestFields["thinking"])
	require.NotNil(t, req.InferenceConfig)
	require.NotNil(t, req.InferenceConfig.MaxTokens)
	assert.Equal(t, 4096, *req.InferenceConfig.MaxTokens)

	_, warnings, _, err = buildRequestWithFamily("amazon.nova-lite-v1:0", "", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")}, ProviderOptions: options,
	})
	require.NoError(t, err)
	assert.Contains(t, warningFeatures(warnings), "budgetTokens")
}

func TestConverseParity_JSONToolModeWithoutResponseFormat(t *testing.T) {
	config := map[string]any{"format": "old", "effort": "high"}
	req, _, meta := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		ProviderOptions: provider.BuildProviderOptions(BedrockOptions{
			StructuredOutputMode:         StructuredOutputModeJSONTool,
			AdditionalModelRequestFields: map[string]any{"output_config": config},
		}),
	})
	assert.False(t, meta.usesJSONResponseTool)
	assert.Equal(t, map[string]any{"effort": "high"}, req.AdditionalModelRequestFields["output_config"])
	assert.Equal(t, "old", config["format"])
}

func TestConverseParity_OpenAIEffort(t *testing.T) {
	for _, tc := range []struct{ id, key string }{
		{"openai.gpt-oss-120b-1:0", "reasoning_effort"},
		{"us.openai.gpt-5.2", "reasoning"},
		{"prefix.middle.openai.gpt-oss-120b", "reasoningConfig"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			req, _, _ := mustBuildRequest(t, tc.id, provider.CallOptions{
				Prompt:          []provider.Message{provider.UserText("hi")},
				ProviderOptions: provider.BuildProviderOptions(BedrockOptions{ReasoningConfig: &ReasoningConfig{MaxReasoningEffort: "medium"}, AdditionalModelRequestFields: map[string]any{"reasoning": map[string]any{"summary": "auto"}}}),
			})
			assert.Contains(t, req.AdditionalModelRequestFields, tc.key)
			if tc.key == "reasoning" {
				assert.Equal(t, map[string]any{"summary": "auto", "effort": "medium"}, req.AdditionalModelRequestFields["reasoning"])
				assert.NotContains(t, req.AdditionalModelRequestFields, "reasoning_effort")
			}
		})
	}
}

func TestConverseParity_ModePrecedenceAndStrictTools(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","additionalProperties":false}`)
	anthropic := provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"structuredOutputMode":"jsonTool","disableParallelToolUse":true}`)}
	strict := true
	for _, tc := range []struct {
		name       string
		bedrock    json.RawMessage
		wantNative bool
	}{
		{name: "anthropic fallback", wantNative: false},
		{name: "bedrock overrides", bedrock: json.RawMessage(`{"structuredOutputMode":"outputFormat"}`), wantNative: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := provider.ProviderOptions{"anthropic": anthropic}
			if tc.bedrock != nil {
				options["amazonBedrock"] = provider.RawProviderOption{Key: "amazonBedrock", Raw: tc.bedrock}
			}
			req, _, meta, err := buildRequestWithFamily("anthropic.claude-sonnet-4-6", "", provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, ProviderOptions: options,
				ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
				Tools:          []provider.Tool{{Type: provider.ToolTypeFunction, Name: "weather", Strict: &strict, InputSchema: schema}},
			})
			require.NoError(t, err)
			assert.Equal(t, !tc.wantNative, meta.usesJSONResponseTool)
			if tc.wantNative {
				assert.NotNil(t, req.AdditionalModelRequestFields["output_config"].(map[string]any)["format"])
			}
			require.NotNil(t, req.ToolConfig)
			assert.Equal(t, &strict, req.ToolConfig.Tools[0].ToolSpec.Strict)
		})
	}
}

func TestConverseParity_ParallelAndSchema(t *testing.T) {
	strict := true
	choice := provider.ToolChoice{Type: provider.ToolChoiceRequired}
	for _, tc := range []struct {
		name, schema string
		wantStrict   bool
	}{
		{"compatible nested", `{"type":"object","additionalProperties":false,"properties":{"nested":{"type":"object","additionalProperties":false}}}`, true},
		{"boolean schema", `true`, true},
		{"incompatible definitions", `{"type":"object","additionalProperties":false,"$defs":{"nested":{"type":"object"}}}`, false},
		{"incompatible nested", `{"type":"object","additionalProperties":false,"properties":{"nested":{"type":"object"}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"disableParallelToolUse":true}`)}}
			req, warnings, _, err := buildRequestWithFamily(testAnthropicModel, "", provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, ProviderOptions: opts,
				ToolChoice: &choice,
				Tools:      []provider.Tool{{Type: provider.ToolTypeFunction, Name: "weather", Strict: &strict, InputSchema: json.RawMessage(tc.schema)}},
			})
			require.NoError(t, err)
			require.NotNil(t, req.ToolConfig)
			assert.Nil(t, req.ToolConfig.ToolChoice)
			assert.Equal(t, tc.wantStrict, req.ToolConfig.Tools[0].ToolSpec.Strict != nil)
			assert.Equal(t, !tc.wantStrict, len(warnings) > 0)
			assert.Equal(t, map[string]any{"type": "any", "disable_parallel_tool_use": true}, req.AdditionalModelRequestFields["tool_choice"])
		})
	}
}

func TestConverseParity_ProviderToolsAndBetaPrecedence(t *testing.T) {
	choice := provider.ToolChoice{Type: provider.ToolChoiceNone}
	tools := []provider.Tool{
		{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250522", Name: "code_execution", InputSchema: json.RawMessage(`{"type":"string"}`)},
		{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	bo := BedrockOptions{AnthropicBeta: []string{"caller-beta"}, AdditionalModelRequestFields: map[string]any{"anthropic_beta": []string{"raw-beta"}}}
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")}, Tools: tools, ToolChoice: &choice,
		ProviderOptions: provider.BuildProviderOptions(bo),
	})
	assert.Empty(t, warnings)
	require.NotNil(t, req.ToolConfig)
	require.Len(t, req.ToolConfig.Tools, 2)
	assert.Nil(t, req.ToolConfig.ToolChoice)
	assert.NotContains(t, req.AdditionalModelRequestFields, "tool_choice")
	assert.Equal(t, []string{"caller-beta", "code-execution-2025-05-22"}, req.AdditionalModelRequestFields["anthropic_beta"])
	assert.JSONEq(t, `{"$schema":"http://json-schema.org/draft-07/schema#","properties":{"code":{"type":"string"}},"required":["code"],"type":"object","additionalProperties":false}`, string(req.ToolConfig.Tools[0].ToolSpec.InputSchema.JSON))

	functionOnly, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, Tools: tools[1:], ToolChoice: &choice})
	assert.Nil(t, functionOnly.ToolConfig)
}

func TestConverseParity_ProviderToolJSONModeParallelChoice(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"}}}`)
	req, _, meta, err := buildRequestWithFamily(testAnthropicModel, "", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		ProviderOptions: provider.ProviderOptions{
			"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`{"structuredOutputMode":"jsonTool"}`)},
			"anthropic":     provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"disableParallelToolUse":true}`)},
		},
		ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
		Tools:          []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250522", Name: "code_execution"}},
	})
	require.NoError(t, err)
	assert.True(t, meta.usesJSONResponseTool)
	require.NotNil(t, req.ToolConfig)
	require.Len(t, req.ToolConfig.Tools, 2)
	assert.Nil(t, req.ToolConfig.ToolChoice)
	assert.Equal(t, map[string]any{"type": "any", "disable_parallel_tool_use": true}, req.AdditionalModelRequestFields["tool_choice"])
	assert.Equal(t, []string{"code-execution-2025-05-22"}, req.AdditionalModelRequestFields["anthropic_beta"])
}

func TestConverseParity_ExplicitParallelFalse(t *testing.T) {
	choice := provider.ToolChoice{Type: provider.ToolChoiceAuto}
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")}, ToolChoice: &choice,
		ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"disableParallelToolUse":false}`)}},
		Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.bash_20241022", Name: "bash"}},
	})
	assert.Equal(t, map[string]any{"type": "auto", "disable_parallel_tool_use": false}, req.AdditionalModelRequestFields["tool_choice"])
}

func TestConverseParity_UnsupportedProviderToolsFiltered(t *testing.T) {
	choice := provider.ToolChoice{Type: provider.ToolChoiceAuto}
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")}, ToolChoice: &choice,
		ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"disableParallelToolUse":true}`)}},
		Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, Name: "web_search", ID: "anthropic.web_search_20260318"}},
	})
	assert.Nil(t, req.ToolConfig)
	assert.NotContains(t, req.AdditionalModelRequestFields, "tool_choice")
	assert.Contains(t, warningFeatures(warnings), "web_search_20260318 tool")
}

func TestConverseParity_ProviderToolCatalog(t *testing.T) {
	require.Len(t, anthropicProviderToolSchemas, 19)
	require.Len(t, anthropicProviderToolBetas, 19)
	for id, beta := range anthropicProviderToolBetas {
		t.Run(id, func(t *testing.T) {
			canonical := anthropicProviderToolSchemas[id]
			require.True(t, json.Valid(canonical))
			pt := prepareTools([]provider.Tool{{Type: provider.ToolTypeProvider, ID: id, Name: "catalog-tool", InputSchema: json.RawMessage(`{"type":"string"}`)}}, nil, testAnthropicModel, true, nil)
			require.Empty(t, pt.warnings)
			require.NotNil(t, pt.toolConfig)
			require.Len(t, pt.toolConfig.Tools, 1)
			assert.JSONEq(t, string(canonical), string(pt.toolConfig.Tools[0].ToolSpec.InputSchema.JSON))
			if beta != "" {
				assert.Equal(t, []string{beta}, pt.betaOrder)
			} else {
				assert.Empty(t, pt.betaOrder)
			}
		})
	}
	for _, id := range []string{"anthropic.web_search_20250305", "anthropic.web_search_20260318", "anthropic.web_fetch_20260318"} {
		t.Run(id+" filtered", func(t *testing.T) {
			assert.NotContains(t, anthropicProviderToolSchemas, id)
			pt := prepareTools([]provider.Tool{{Type: provider.ToolTypeProvider, ID: id, Name: "web"}}, nil, testAnthropicModel, true, nil)
			assert.Nil(t, pt.toolConfig)
			require.Len(t, pt.warnings, 1)
		})
	}
	assert.JSONEq(t, `{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"command":{"type":"string"},"restart":{"type":"boolean"}},"required":["command"],"additionalProperties":false}`, string(anthropicProviderToolSchemas["anthropic.bash_20241022"]))
	assert.Equal(t, "code-execution-2025-08-25", anthropicProviderToolBetas["anthropic.code_execution_20250825"])
	assert.Equal(t, "advisor-tool-2026-03-01", anthropicProviderToolBetas["anthropic.advisor_20260301"])
}

func TestConverseParity_InvalidAnthropicOptions(t *testing.T) {
	for _, raw := range []string{`{"disableParallelToolUse":"yes"}`, `{"structuredOutputMode":42}`} {
		_, _, _, err := buildRequestWithFamily(testAnthropicModel, "", provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, ProviderOptions: provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(raw)},
		}})
		require.Error(t, err)
	}
}

func TestConverseParity_DocumentNames(t *testing.T) {
	for _, tc := range []struct{ filename, want string }{
		{"John's  report.txt", "Johns report"},
		{strings.Repeat("a", 220) + ".pdf", strings.Repeat("a", 200)},
		{".txt", "document-1"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			doc, err := buildDocumentBlock("text/plain", tc.filename, "aGk=", nil, new(int))
			require.NoError(t, err)
			assert.Equal(t, tc.want, doc.Name)
		})
	}
}
