package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cachedMessage(role provider.Role, parts ...provider.ContentPart) provider.Message {
	return provider.Message{Role: role, Content: parts, ProviderOptions: provider.BuildProviderOptions(BedrockOptions{CachePoint: &CachePoint{Type: "default", TTL: "5m"}})}
}

func TestConvertPrompt_CachePointRequiresSourceContent(t *testing.T) {
	call := provider.ToolCallPart("c1", "lookup", json.RawMessage(`{}`))
	result := provider.ToolResultPart("c1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"})
	for _, tc := range []struct {
		name        string
		prompt      []provider.Message
		tools       bool
		want        string
		wantWarning bool
	}{
		{"disabled_final_tool_result", []provider.Message{provider.UserText("question"), provider.NewAssistantMessage(provider.TextPart("checking"), call), cachedMessage(provider.RoleTool, result)}, false, `[{"role":"user","content":[{"text":"question"}]},{"role":"assistant","content":[{"text":"checking"}]}]`, true},
		{"disabled_tools_before_grouping", []provider.Message{provider.UserText("question"), cachedMessage(provider.RoleAssistant, call), cachedMessage(provider.RoleTool, result), provider.UserText("next")}, false, `[{"role":"user","content":[{"text":"question"},{"text":"next"}]}]`, true},
		{"unsigned_reasoning", []provider.Message{provider.UserText("question"), cachedMessage(provider.RoleAssistant, provider.ReasoningPart("private")), provider.UserText("next")}, true, `[{"role":"user","content":[{"text":"question"}]},{"role":"user","content":[{"text":"next"}]}]`, false},
		{"unsigned_reasoning_tools_disabled", []provider.Message{provider.UserText("question"), cachedMessage(provider.RoleAssistant, provider.ReasoningPart("private"))}, false, `[{"role":"user","content":[{"text":"question"}]}]`, false},
		{"assistant_approval", []provider.Message{provider.UserText("question"), cachedMessage(provider.RoleAssistant, provider.ContentPart{Type: provider.ContentPartTypeToolApprovalRequest})}, true, `[{"role":"user","content":[{"text":"question"}]}]`, false},
		{"tool_approval_cannot_borrow_previous_content", []provider.Message{provider.UserText("question"), cachedMessage(provider.RoleTool, provider.ContentPart{Type: provider.ContentPartTypeToolApprovalResponse})}, true, `[{"role":"user","content":[{"text":"question"}]}]`, false},
		{"user_approval", []provider.Message{cachedMessage(provider.RoleUser, provider.ContentPart{Type: provider.ContentPartTypeToolApprovalResponse}), provider.UserText("question")}, false, `[{"role":"user","content":[{"text":"question"}]}]`, false},
		{"assistant_cannot_borrow_previous_content", []provider.Message{provider.NewAssistantMessage(provider.TextPart("answer")), cachedMessage(provider.RoleAssistant, provider.ReasoningPart("private"))}, true, `[{"role":"assistant","content":[{"text":"answer"}]}]`, false},
		{"partial_content_keeps_cache", []provider.Message{cachedMessage(provider.RoleAssistant, provider.TextPart("answer"), call)}, false, `[{"role":"assistant","content":[{"text":"answer"},{"cachePoint":{"type":"default","ttl":"5m"}}]}]`, true},
		{"valid_user_cache", []provider.Message{cachedMessage(provider.RoleUser, provider.TextPart("question"), provider.ContentPart{Type: provider.ContentPartTypeToolApprovalResponse})}, true, `[{"role":"user","content":[{"text":"question"},{"cachePoint":{"type":"default","ttl":"5m"}}]}]`, false},
		{"active_tool_cache", []provider.Message{cachedMessage(provider.RoleAssistant, call), cachedMessage(provider.RoleTool, result)}, true, `[{"role":"assistant","content":[{"toolUse":{"toolUseId":"c1","name":"lookup","input":{}}},{"cachePoint":{"type":"default","ttl":"5m"}}]},{"role":"user","content":[{"toolResult":{"toolUseId":"c1","content":[{"text":"ok"}]}},{"cachePoint":{"type":"default","ttl":"5m"}}]}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := json.Marshal(tc.prompt)
			require.NoError(t, err)
			got, warnings, err := convertPrompt(tc.prompt, false, tc.tools)
			require.NoError(t, err)
			encoded, err := json.Marshal(got.Messages)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
			if tc.wantWarning {
				require.Len(t, warnings, 1)
				assert.Equal(t, "toolContent", warnings[0].Feature)
			} else {
				assert.Empty(t, warnings)
			}
			after, err := json.Marshal(tc.prompt)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
		})
	}
}

func TestBuildRequest_CachePointToolAvailability(t *testing.T) {
	for _, tc := range []struct {
		name      string
		tools     []provider.Tool
		choice    *provider.ToolChoice
		format    *provider.ResponseFormat
		options   provider.ProviderOptions
		wantTools bool
	}{
		{name: "no_tools"},
		{name: "unsupported_provider_tool", tools: []provider.Tool{{Type: provider.ToolTypeProvider, Name: "web_search", ID: "anthropic.web_search_20260318"}}},
		{name: "tool_choice_none", tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}}, choice: &provider.ToolChoice{Type: provider.ToolChoiceNone}},
		{name: "json_tool", format: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object"}`)}, options: provider.BuildProviderOptions(BedrockOptions{StructuredOutputMode: StructuredOutputModeJSONTool}), wantTools: true},
		{name: "native_json_without_tools", format: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object"}`)}, options: provider.BuildProviderOptions(BedrockOptions{StructuredOutputMode: StructuredOutputModeOutputFormat})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prompt := []provider.Message{provider.UserText("question"), cachedMessage(provider.RoleAssistant, provider.ToolCallPart("c1", "lookup", json.RawMessage(`{}`))), cachedMessage(provider.RoleTool, provider.ToolResultPart("c1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"})), provider.UserText("next")}
			req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{Prompt: prompt, Tools: tc.tools, ToolChoice: tc.choice, ResponseFormat: tc.format, ProviderOptions: tc.options})
			if tc.wantTools {
				require.Len(t, req.Messages, 3)
				require.Len(t, req.Messages[1].Content, 2)
				assert.NotNil(t, req.Messages[1].Content[0].ToolUse)
				assert.NotNil(t, req.Messages[1].Content[1].CachePoint)
				assert.NotContains(t, warningFeatures(warnings), "toolContent")
			} else {
				require.Len(t, req.Messages, 1)
				require.Len(t, req.Messages[0].Content, 2)
				assert.Equal(t, "question", req.Messages[0].Content[0].Text)
				assert.Equal(t, "next", req.Messages[0].Content[1].Text)
				assert.Contains(t, warningFeatures(warnings), "toolContent")
			}
		})
	}
}

func TestConvertPrompt_CachePointFilteringBoundaries(t *testing.T) {
	t.Run("system_source_ownership", func(t *testing.T) {
		got, warnings, err := convertPrompt([]provider.Message{cachedMessage(provider.RoleSystem, provider.TextPart("system")), cachedMessage(provider.RoleSystem, provider.ReasoningPart("ignored")), provider.UserText("question")}, false, true)
		require.NoError(t, err)
		require.Len(t, got.System, 2)
		assert.Equal(t, "system", got.System[0].Text)
		assert.NotNil(t, got.System[1].CachePoint)
		require.Len(t, warnings, 1)
		assert.Equal(t, "systemContentPart", warnings[0].Feature)
	})
	t.Run("signed_reasoning_retains_cache", func(t *testing.T) {
		part := provider.ReasoningPart(" signed \n")
		part.ProviderOptions = provider.BuildProviderOptions(ReasoningMetadata{Signature: "sig"})
		got, warnings, err := convertPrompt([]provider.Message{cachedMessage(provider.RoleAssistant, part)}, false, false)
		require.NoError(t, err)
		assert.Empty(t, warnings)
		require.Len(t, got.Messages, 1)
		require.Len(t, got.Messages[0].Content, 2)
		assert.Equal(t, " signed \n", got.Messages[0].Content[0].ReasoningContent.ReasoningText.Text)
		assert.NotNil(t, got.Messages[0].Content[1].CachePoint)
	})
	t.Run("disabled_tool_tail_updates_prefill_position", func(t *testing.T) {
		got, _, err := convertPrompt([]provider.Message{provider.NewAssistantMessage(provider.TextPart("  prefill  "), provider.ToolCallPart("c1", "lookup", json.RawMessage(`{}`))), cachedMessage(provider.RoleTool, provider.ToolResultPart("c1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}))}, false, false)
		require.NoError(t, err)
		require.Len(t, got.Messages, 1)
		assert.Equal(t, "prefill", got.Messages[0].Content[0].Text)
	})
	t.Run("ignored_approval_preserves_source_prefill_position", func(t *testing.T) {
		got, _, err := convertPrompt([]provider.Message{provider.NewAssistantMessage(provider.TextPart("  prefill  "), provider.ContentPart{Type: provider.ContentPartTypeToolApprovalRequest})}, false, true)
		require.NoError(t, err)
		require.Len(t, got.Messages, 1)
		assert.Equal(t, "  prefill  ", got.Messages[0].Content[0].Text)
	})
	t.Run("dropped_tool_message_options_are_not_parsed", func(t *testing.T) {
		msg := provider.NewToolMessage(provider.ToolResultPart("c1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}))
		msg.ProviderOptions = provider.ProviderOptions{"bedrock": provider.RawProviderOption{Key: "bedrock", Raw: json.RawMessage(`{"cachePoint":false}`)}}
		got, warnings, err := convertPrompt([]provider.Message{provider.UserText("question"), msg}, false, false)
		require.NoError(t, err)
		require.Len(t, got.Messages, 1)
		assert.Contains(t, warningFeatures(warnings), "toolContent")
		_, _, err = convertPrompt([]provider.Message{msg}, false, true)
		assert.Error(t, err)
		msg = cachedMessage(provider.RoleAssistant, provider.ReasoningPart("unsigned"))
		msg.ProviderOptions = provider.ProviderOptions{"bedrock": provider.RawProviderOption{Key: "bedrock", Raw: json.RawMessage(`{"cachePoint":false}`)}}
		_, _, err = convertPrompt([]provider.Message{msg}, false, true)
		assert.Error(t, err)
	})
}
