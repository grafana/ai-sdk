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
