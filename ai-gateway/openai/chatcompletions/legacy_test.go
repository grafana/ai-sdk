package chatcompletions

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyFunctions_Normalization(t *testing.T) {
	r := requestWith(t, `,"functions":[{"name":"weather"}],"function_call":{"name":"weather"}`)
	assert.True(t, r.legacyFunctions)
	require.Len(t, r.options.Tools, 1)
	assert.Equal(t, "weather", r.options.ToolChoice.ToolName)
	require.NotNil(t, r.Parallel)
	assert.False(t, *r.Parallel)
	assert.JSONEq(t, `{"type":"object","properties":{},"additionalProperties":false}`, string(r.options.Tools[0].InputSchema))
	for _, content := range []string{`"sunny"`, `null`} {
		r, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"assistant","content":null,"function_call":{"name":"weather","arguments":"{}"}},{"role":"function","name":"weather","content":` + content + `}]}`))
		require.NoError(t, err)
		assert.Equal(t, provider.RoleTool, r.options.Prompt[1].Role)
		assert.Equal(t, r.options.Prompt[0].Content[0].ToolCallID, r.options.Prompt[1].Content[0].ToolCallID)
	}
	r, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"assistant","tool_calls":[{"id":"chat_legacy_2","type":"function","function":{"name":"weather","arguments":"{}"}}]},{"role":"tool","tool_call_id":"chat_legacy_2","content":"sunny"},{"role":"assistant","function_call":{"name":"weather","arguments":"{}"}},{"role":"function","name":"weather","content":"sunny"}]}`))
	require.NoError(t, err)
	assert.NotEqual(t, r.options.Prompt[0].Content[0].ToolCallID, r.options.Prompt[2].Content[0].ToolCallID)
	assert.Equal(t, r.options.Prompt[2].Content[0].ToolCallID, r.options.Prompt[3].Content[0].ToolCallID)
	for _, extra := range []string{
		`,"functions":[{"name":"weather"}],"tools":[]`,
		`,"functions":[{"name":"weather"}],"tool_choice":"auto"`,
		`,"functions":[{"name":"weather"}],"parallel_tool_calls":true`,
		`,"functions":[{"name":"weather"}],"function_call":{"name":"other"}`,
		`,"function_call":"required"`,
		`,"messages":[{"role":"user","content":"hi","function_call":{"name":"weather","arguments":"{}"}}]`,
		`,"messages":[{"role":"function","name":"weather","content":"sunny"}]`,
		`,"messages":[{"role":"assistant","function_call":{"name":"weather","arguments":"{}"}},{"role":"function","name":"other","content":"sunny"}]`,
		`,"messages":[{"role":"assistant","function_call":{"name":"weather","arguments":"{}"},"tool_calls":[{"id":"x","type":"function","function":{"name":"weather","arguments":"{}"}}]}]`,
	} {
		_, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"user","content":"hi"}]` + extra + `}`))
		assert.Error(t, err, extra)
	}
}

func TestLegacyFunctions_Output(t *testing.T) {
	r := requestWith(t, `,"functions":[{"name":"weather"}]`)
	for _, reason := range []provider.UnifiedFinishReason{provider.FinishReasonToolCalls, provider.FinishReasonLength} {
		result := textResult()
		result.Content = []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "c", ToolName: "weather", Input: json.RawMessage(`{}`)}}
		result.FinishReason.Unified = reason
		out, err := mapGenerate(result, r, "id", "model", 1)
		require.NoError(t, err)
		require.NotNil(t, out.Choices[0].Message.FunctionCall)
		assert.Empty(t, out.Choices[0].Message.ToolCalls)
		want := "function_call"
		if reason == provider.FinishReasonLength {
			want = "length"
		}
		assert.Equal(t, want, out.Choices[0].Finish)
	}
	d, err := legacyDelta(delta{ToolCalls: []toolDelta{{Index: 0, ID: "call", Type: kindFunction, Function: functionDelta{Name: "weather", Arguments: ptr("{")}}}})
	require.NoError(t, err)
	assert.Empty(t, d.ToolCalls)
	assert.Equal(t, "weather", d.FunctionCall.Name)
	assert.Equal(t, "{", *d.FunctionCall.Arguments)
	_, err = legacyDelta(delta{ToolCalls: []toolDelta{{Index: 1}}})
	require.Error(t, err)
}
