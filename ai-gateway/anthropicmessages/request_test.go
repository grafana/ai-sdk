package anthropicmessages

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapRequest_TextToolsAndErrors(t *testing.T) {
	body := []byte(`{"model":"public-model","max_tokens":512,"system":"Be concise","messages":[{"role":"user","content":"find weather"},{"role":"assistant","content":[{"type":"tool_use","id":"call-1","name":"weather","input":{"city":"Oslo"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":"snow"}]}],"tools":[{"name":"weather","description":"lookup","input_schema":{"type":"object"}}],"tool_choice":{"type":"tool","name":"weather"},"top_k":2,"stop_sequences":["done"]}`)
	input, options, err := mapRequest(body)
	require.NoError(t, err)
	assert.Equal(t, "public-model", input.Model)
	assert.Equal(t, 512, *options.MaxOutputTokens)
	assert.Equal(t, provider.ToolChoiceTool, options.ToolChoice.Type)
	assert.Equal(t, "weather", options.ToolChoice.ToolName)
	assert.Nil(t, options.ProviderOptions)
	assert.Len(t, options.Prompt, 4)
	assert.Equal(t, provider.RoleSystem, options.Prompt[0].Role)
	assert.Equal(t, "Be concise", options.Prompt[0].Content[0].Text)
	assert.Equal(t, "find weather", options.Prompt[1].Content[0].Text)
	assert.Equal(t, json.RawMessage(`{"city":"Oslo"}`), options.Prompt[2].Content[0].Input)
	assert.Equal(t, provider.RoleTool, options.Prompt[3].Role)
	assert.Equal(t, "call-1", options.Prompt[3].Content[0].ToolCallID)
	assert.Equal(t, "weather", options.Prompt[3].Content[0].ToolName)
	assert.Equal(t, "snow", options.Prompt[3].Content[0].Output.Text)
}

func TestMapRequest_PreservesAnthropicToolChoiceOptions(t *testing.T) {
	_, options, err := mapRequest([]byte(`{"model":"m","max_tokens":2,"messages":[{"role":"user","content":"hello"}],"tool_choice":{"type":"auto","disable_parallel_tool_use":true}}`))
	require.NoError(t, err)
	var decoded map[string]any
	raw := options.ProviderOptions["anthropic"].(provider.RawProviderOption).Raw
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, true, decoded["disableParallelToolUse"])
}

func TestMapRequest_RejectsUnsupportedContentBeforeExecution(t *testing.T) {
	for _, body := range []string{
		`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA=="}}]}]}`,
		`{"model":"m","max_tokens":10,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"hidden","signature":"sig"}]}]}`,
		`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"x","content":[{"type":"image","source":{}}]}]}]}`,
	} {
		_, _, err := mapRequest([]byte(body))
		assert.ErrorIs(t, err, errUnsupportedRequest)
	}
}

func TestMapRequest_RejectsUnmatchedOrReusedToolResults(t *testing.T) {
	for _, body := range []string{
		`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"missing","content":"done"}]}]}`,
		`{"model":"m","max_tokens":10,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call","name":"weather","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call","content":"one"},{"type":"tool_result","tool_use_id":"call","content":"two"}]}]}`,
	} {
		_, _, err := mapRequest([]byte(body))
		assert.ErrorIs(t, err, errInvalidRequest)
	}
}

func TestMapRequest_OrdinaryJSONFieldMatchingAndAdditiveFields(t *testing.T) {
	_, options, err := mapRequest([]byte(`{"MODEL":"m","max_tokens":1,"future_addition":true,"messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	assert.Equal(t, "hi", options.Prompt[0].Content[0].Text)
}

func TestResponseMessage_StopReasonsAndUsage(t *testing.T) {
	input, cacheRead, cacheWrite, text, reasoning := 8, 2, 1, 3, 2
	result := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "done"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{NoCache: &input, CacheRead: &cacheRead, CacheWrite: &cacheWrite},
			OutputTokens: provider.OutputTokenUsage{Text: &text, Reasoning: &reasoning},
		},
	}
	message, ok := responseMessage("public-name", result)
	require.True(t, ok)
	assert.Equal(t, "public-name", message.Model)
	assert.Equal(t, "end_turn", *message.StopReason)
	assert.Equal(t, usage{InputTokens: 8, OutputTokens: 5, CacheReadInputTokens: 2, CacheCreationInputTokens: 1}, message.Usage)

	for _, tc := range []struct {
		reason provider.FinishReason
		want   string
	}{
		{reason: provider.FinishReason{Unified: provider.FinishReasonToolCalls}, want: "tool_use"},
		{reason: provider.FinishReason{Unified: provider.FinishReasonLength}, want: "max_tokens"},
		{reason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "pause_turn"}, want: "pause_turn"},
		{reason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "stop_sequence"}, want: "stop_sequence"},
	} {
		reason, valid := stopReason(tc.reason)
		assert.True(t, valid)
		assert.Equal(t, tc.want, reason)
	}
}

func TestHasUnsupportedHeaders(t *testing.T) {
	for _, header := range []string{"Anthropic-Beta", "Anthropic-User-Profile-Id", "Anthropic-Workspace-Id"} {
		assert.True(t, hasUnsupportedHeaders(map[string][]string{header: {"requested"}}))
	}
	assert.False(t, hasUnsupportedHeaders(map[string][]string{"Anthropic-Version": {"2023-06-01"}}))
}

func TestMapRequest_NativeToolExamplesAndEmptyResults(t *testing.T) {
	_, options, err := mapRequest([]byte(`{"model":"m","max_tokens":10,"tools":[{"name":"weather","input_schema":{"type":"object"},"input_examples":[{"city":"Oslo"},{"input":123}]}],"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call","name":"weather","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call"}]}]}`))
	require.NoError(t, err)
	require.Len(t, options.Tools, 1)
	require.Len(t, options.Tools[0].InputExamples, 2)
	assert.JSONEq(t, `{"city":"Oslo"}`, string(options.Tools[0].InputExamples[0].Input))
	assert.JSONEq(t, `{"input":123}`, string(options.Tools[0].InputExamples[1].Input))
	assert.Empty(t, options.Prompt[1].Content[0].Output.Text)
}

func TestResponseMessage_PreservesToolNumbers(t *testing.T) {
	for _, input := range []string{`{"id":9007199254740993}`, `{"value":1e400}`} {
		t.Run(input, func(t *testing.T) {
			message, ok := responseMessage("m", &provider.GenerateResult{
				Content:      []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "lookup", Input: json.RawMessage(input)}},
				FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls},
			})
			require.True(t, ok)
			encoded, err := json.Marshal(message)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), `"input":`+input)
		})
	}
}
