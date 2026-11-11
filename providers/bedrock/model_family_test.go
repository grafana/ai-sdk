package bedrock

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelFamily_StructuredOutputSupport(t *testing.T) {
	tests := []struct {
		modelID      string
		native       bool
		strict       bool
		known        bool
		capabilitySO bool
	}{
		{modelID: "anthropic.claude-3-7-sonnet-20250219-v1:0", native: true, strict: true},
		{modelID: "anthropic.claude-3-haiku-20240307-v1:0", native: true, strict: true, known: true},
		{modelID: "anthropic.claude-instant-v1", native: true, strict: true},
		{modelID: "anthropic.claude-v2:1", native: true, strict: true},
		{modelID: "anthropic.claude-sonnet-4-20250514-v1:0", native: true, strict: true, known: true},
		{modelID: "us.anthropic.claude-opus-4-1-20250805-v1:0", native: true, strict: true, known: true, capabilitySO: true},
		{modelID: "anthropic.claude-sonnet-4-5-20250929-v1:0", native: true, strict: true, known: true, capabilitySO: true},
		{modelID: "anthropic.claude-opus-4-6-v1", native: true, strict: true, known: true, capabilitySO: true},
		{modelID: "anthropic.claude-sonnet-4-6-v1", strict: true, known: true, capabilitySO: true},
		{modelID: "anthropic.claude-haiku-4-5-20251001-v1:0", strict: true, known: true, capabilitySO: true},
		{modelID: "anthropic.claude-opus-4-7", known: true, capabilitySO: true},
		{modelID: "anthropic.claude-opus-5", known: true, capabilitySO: true},
		{modelID: "anthropic.claude-sonnet-5-5", known: true, capabilitySO: true},
		{modelID: "us.anthropic.claude-haiku-5-5", known: true, capabilitySO: true},
		{modelID: "us.anthropic.claude-opus-4-9-v1:0", known: true},
		{modelID: "us.anthropic.claude-opus-6-v1:0", capabilitySO: true},
		{modelID: "global.anthropic.claude-future-v1:0", capabilitySO: true},
		{modelID: "anthropic.claude-opus-4-60-v1", known: true, capabilitySO: true},
		{modelID: "meta.llama3-70b-instruct-v1:0", native: true, strict: true},
		{modelID: "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/custom", native: true, strict: true},
	}
	for _, tc := range tests {
		t.Run(tc.modelID, func(t *testing.T) {
			assert.Equal(t, tc.native, supportsNativeStructuredOutput(tc.modelID), "native")
			assert.Equal(t, tc.strict, supportsStrictTools(tc.modelID), "strict")
			assert.Equal(t, tc.known, isKnownClaudeModel(tc.modelID), "known")
			assert.Equal(t, tc.capabilitySO, supportsStructuredOutputCapability(tc.modelID), "capability")
		})
	}
}
