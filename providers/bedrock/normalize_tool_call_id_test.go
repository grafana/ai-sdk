package bedrock

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeToolCallID(t *testing.T) {
	cases := []struct {
		name     string
		id       string
		mistral  bool
		expected string
	}{
		{"non-mistral passes through", "tooluse_bpe71yCfRu2b5i-nKGDr5g", false, "tooluse_bpe71yCfRu2b5i-nKGDr5g"},
		{"mistral takes first 9 alphanumeric", "tooluse_bpe71yCfRu2b5i-nKGDr5g", true, "toolusebp"},
		{"mistral handles short id", "abc-123", true, "abc123"},
		{"mistral handles empty", "", true, ""},
		{"mistral handles exactly 9 alphanumeric", "abc123XYZ", true, "abc123XYZ"},
		{"mistral strips underscores", "tool_use_id", true, "tooluseid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, normalizeToolCallID(tc.id, tc.mistral))
		})
	}
}

func TestModelFamilyDetection(t *testing.T) {
	cases := []struct {
		modelID  string
		isAnth   bool
		isOpenAI bool
		isMistr  bool
	}{
		{"anthropic.claude-sonnet-4-5-20250929-v1:0", true, false, false},
		{"us.anthropic.claude-haiku-4-5-20251001-v1:0", true, false, false},
		{"openai.gpt-oss-20251101-v1:0", false, true, false},
		{"us.openai.gpt-oss-20251101-v1:0", false, true, false},
		{"mistral.mistral-large-2407-v1:0", false, false, true},
		{"amazon.nova-lite-v1:0", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.modelID, func(t *testing.T) {
			assert.Equal(t, tc.isAnth, isAnthropicModel(tc.modelID), "isAnthropicModel")
			assert.Equal(t, tc.isOpenAI, isOpenAIModel(tc.modelID), "isOpenAIModel")
			assert.Equal(t, tc.isMistr, isMistralModel(tc.modelID), "isMistralModel")
		})
	}
}

func TestSupportsNativeStructuredOutput(t *testing.T) {
	cases := []struct {
		modelID  string
		expected bool
	}{
		{"anthropic.claude-sonnet-4-5-20250929-v1:0", true},
		{"anthropic.claude-haiku-4-5-20251001-v1:0", true},
		{"anthropic.claude-opus-4-1-20250805-v1:0", true},
		{"us.anthropic.claude-future-9-20990101-v1:0", true},
		{"anthropic.claude-3-haiku-20240307-v1:0", false},
		{"us.anthropic.claude-3-7-sonnet-20250219-v1:0", false},
		{"mistral.mistral-large-2407-v1:0", false},
		{"amazon.nova-lite-v1:0", false},
	}
	for _, tc := range cases {
		t.Run(tc.modelID, func(t *testing.T) {
			assert.Equal(t, tc.expected, supportsNativeStructuredOutput(tc.modelID))
		})
	}
}

func TestAnthropicReasoningCapabilities(t *testing.T) {
	cases := []struct {
		modelID   string
		maxTokens int
		adaptive  bool
	}{
		{"anthropic.claude-opus-4-8-v1:0", 128000, true},
		{"us.anthropic.claude-opus-4-7-v1:0", 128000, true},
		{"anthropic.claude-opus-4-6-v1:0", 128000, true},
		{"anthropic.claude-sonnet-4-6-v1:0", 128000, true},
		{"anthropic.claude-fable-5-v1:0", 128000, true},
		{"anthropic.claude-sonnet-5-v1:0", 128000, true},
		{"anthropic.claude-sonnet-4-5-20250929-v1:0", 64000, false},
		{"anthropic.claude-opus-4-1-v1:0", 32000, false},
		{"anthropic.claude-sonnet-4-0-v1:0", 64000, false},
		{"anthropic.claude-opus-4-0-v1:0", 32000, false},
		{"anthropic.claude-3-haiku-20240307-v1:0", 4096, false},
		{"us.anthropic.claude-3-7-sonnet-20250219-v1:0", 4096, false},
		{"us.anthropic.claude-future-9-20990101-v1:0", 128000, true},
		{"anthropic.unknown-model-v1:0", 4096, false},
		{"amazon.claude-sonnet-4-6-v1:0", 4096, false},
	}
	for _, tc := range cases {
		t.Run(tc.modelID, func(t *testing.T) {
			capabilities := getAnthropicReasoningCapabilities(tc.modelID)
			assert.Equal(t, tc.maxTokens, capabilities.maxOutputTokens)
			assert.Equal(t, tc.adaptive, capabilities.supportsAdaptiveThinking)
			assert.Equal(t, tc.adaptive, supportsAdaptiveThinking(tc.modelID))
		})
	}
}
