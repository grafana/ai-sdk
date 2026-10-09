package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallOptions_WireRoundTrip(t *testing.T) {
	intPtr := func(i int) *int { return &i }
	floatPtr := func(f float64) *float64 { return &f }
	full := CallOptions{
		Prompt: []Message{
			NewSystemMessage("be helpful"),
			NewUserMessage(
				ContentPart{Type: ContentPartTypeText, Text: "describe"},
				ContentPart{Type: ContentPartTypeFile, MediaType: "image/png", Data: &DataContent{URL: "https://example.com/x.png"}},
			),
			NewAssistantMessage(
				ContentPart{Type: ContentPartTypeReasoning, Text: "thinking"},
				ContentPart{Type: ContentPartTypeToolCall, ToolCallID: "tc_1", ToolName: "search", Input: json.RawMessage(`{"q":"go"}`)},
			),
			NewToolMessage(
				ContentPart{
					Type: ContentPartTypeToolResult, ToolCallID: "tc_1", ToolName: "search",
					Output: &ToolResultOutput{Type: ToolOutputText, Text: "ok"},
				},
			),
		},
		Tools: []Tool{
			{
				Type:        ToolTypeFunction,
				Name:        "search",
				Description: "Searches the web",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
				InputExamples: []InputExample{
					{Input: json.RawMessage(`{"q":"hello"}`)},
				},
				Strict: boolPtr(false),
				ProviderOptions: ProviderOptions{
					"anthropic": RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"cache":"ephemeral"}`)},
				},
			},
			{
				Type: ToolTypeProvider,
				Name: "web_search",
				ID:   "anthropic.web_search_20250305",
				Args: map[string]json.RawMessage{
					"maxUses": json.RawMessage(`5`),
				},
			},
		},
		ToolChoice:       &ToolChoice{Type: ToolChoiceTool, ToolName: "search"},
		MaxOutputTokens:  intPtr(1024),
		Temperature:      floatPtr(0.7),
		TopP:             floatPtr(0.95),
		TopK:             intPtr(40),
		PresencePenalty:  floatPtr(0.1),
		FrequencyPenalty: floatPtr(0.2),
		StopSequences:    []string{"END", "\n\n"},
		ResponseFormat:   &ResponseFormat{Type: ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object"}`), Name: "result", Description: "the answer"},
		Seed:             intPtr(42),
		Reasoning:        ReasoningHigh,
		IncludeRawChunks: true,
		Headers:          map[string]string{"X-Trace-ID": "abc"},
		ProviderOptions: ProviderOptions{
			"anthropic": RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"budget":1024}}`)},
		},
	}

	data, err := json.Marshal(full)
	require.NoError(t, err)

	var decoded CallOptions
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, full, decoded)
}

func TestCallOptions_ReasoningJSON(t *testing.T) {
	t.Run("provider default is the omitted zero value", func(t *testing.T) {
		assert.Equal(t, ReasoningEffort(""), ReasoningProviderDefault)

		data, err := json.Marshal(CallOptions{Reasoning: ReasoningProviderDefault})
		require.NoError(t, err)
		assert.JSONEq(t, `{}`, string(data))
	})

	t.Run("operational value is explicit", func(t *testing.T) {
		data, err := json.Marshal(CallOptions{Reasoning: ReasoningHigh})
		require.NoError(t, err)
		assert.JSONEq(t, `{"reasoning":"high"}`, string(data))
	})
}

func TestCallOptions_EmptyJSON(t *testing.T) {
	var opts CallOptions
	data, err := json.Marshal(opts)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(data))
}

func TestValidateTools(t *testing.T) {
	for _, tc := range []struct {
		name      string
		tool      Tool
		wantError bool
	}{
		{name: "nil provider args default to empty", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search"}},
		{name: "selected empty provider args", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Args: map[string]json.RawMessage{}}},
		{name: "nested provider args", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Args: map[string]json.RawMessage{"filters": json.RawMessage(`{"names":[null,0,false]}`)}}},
		{name: "malformed provider args", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Args: map[string]json.RawMessage{"maxUses": json.RawMessage(`{`)}}, wantError: true},
		{name: "empty provider arg value", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Args: map[string]json.RawMessage{"maxUses": nil}}, wantError: true},
		{name: "provider description", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Description: "forbidden"}, wantError: true},
		{name: "provider input schema", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", InputSchema: json.RawMessage(`{}`)}, wantError: true},
		{name: "provider examples", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", InputExamples: []InputExample{}}, wantError: true},
		{name: "provider strict false", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Strict: boolPtr(false)}, wantError: true},
		{name: "provider options even if empty", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", ProviderOptions: ProviderOptions{}}, wantError: true},
		{name: "function id", tool: Tool{Type: ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{}`), ID: "anthropic.web_search"}, wantError: true},
		{name: "function args", tool: Tool{Type: ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{}`), Args: map[string]json.RawMessage{}}, wantError: true},
		{name: "function strict false and options", tool: Tool{Type: ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{}`), Strict: boolPtr(false), ProviderOptions: ProviderOptions{"anthropic": RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{}`)}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTools([]Tool{tc.tool})
			if tc.wantError {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTool_InputExamplesJSONPresence(t *testing.T) {
	tests := []struct {
		name     string
		examples []InputExample
		expected string
	}{
		{name: "omitted", expected: `{"type":"function","name":"weather"}`},
		{name: "explicit empty", examples: []InputExample{}, expected: `{"type":"function","name":"weather","inputExamples":[]}`},
		{name: "populated", examples: []InputExample{{Input: json.RawMessage(`{"city":"Rio"}`)}}, expected: `{"type":"function","name":"weather","inputExamples":[{"input":{"city":"Rio"}}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(Tool{Type: ToolTypeFunction, Name: "weather", InputExamples: tc.examples})
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(data))

			var decoded Tool
			require.NoError(t, json.Unmarshal(data, &decoded))
			if tc.examples == nil {
				assert.Nil(t, decoded.InputExamples)
			} else {
				assert.NotNil(t, decoded.InputExamples)
				assert.Len(t, decoded.InputExamples, len(tc.examples))
			}
		})
	}
}
