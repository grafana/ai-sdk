package openai

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetModelCapabilities(t *testing.T) {
	tests := []struct {
		modelID           string
		isReasoning       bool
		systemMode        string
		supportsFlex      bool
		supportsPriority  bool
		supportsNonReason bool
	}{
		{"gpt-4o", false, "system", false, true, false},
		{"gpt-4.1", false, "system", false, true, false},
		{"gpt-5", true, "developer", true, true, false},
		{"gpt-5-mini", true, "developer", true, true, false},
		{"gpt-5-nano", true, "developer", true, false, false},
		{"gpt-5-chat-latest", false, "system", false, false, false},
		{"gpt-5.1", true, "developer", true, true, true},
		{"gpt-5.6", true, "developer", true, true, true},
		{"gpt-5.6-luna", true, "developer", true, true, true},
		{"gpt-5.99-chat-latest", true, "developer", true, true, true},
		{"gpt-99", true, "developer", true, true, true},
		{"gpt-99-mini", true, "developer", true, true, true},
		{"gpt-99-nano", true, "developer", true, false, true},
		{"ft:gpt-99:org:custom:abc", false, "system", false, false, false},
		{"acme-gpt-99-proxy", false, "system", false, false, false},
		{"o1", true, "developer", false, false, false},
		{"o3", true, "developer", true, true, false},
		{"o4", true, "developer", true, true, false},
		{"o99-2099-01-01", true, "developer", true, true, false},
		{"unknown-model", false, "system", false, false, false},
	}

	for _, tc := range tests {
		t.Run(tc.modelID, func(t *testing.T) {
			caps := getModelCapabilities(tc.modelID)
			assert.Equal(t, tc.isReasoning, caps.isReasoningModel, "isReasoningModel")
			assert.Equal(t, tc.systemMode, caps.systemMessageMode, "systemMessageMode")
			assert.Equal(t, tc.supportsFlex, caps.supportsFlexProcessing, "supportsFlexProcessing")
			assert.Equal(t, tc.supportsPriority, caps.supportsPriorityProcessing, "supportsPriorityProcessing")
			assert.Equal(t, tc.supportsNonReason, caps.supportsNonReasoningParameters, "supportsNonReasoningParameters")
		})
	}
}

func TestModelIDs(t *testing.T) {
	ids := ModelIDs()
	assert.Len(t, ids, 67)
	assert.Contains(t, ids, "gpt-4o")
	assert.Contains(t, ids, "gpt-4o-2024-11-20")
	assert.Contains(t, ids, "gpt-4o-audio-preview")
	assert.Contains(t, ids, "gpt-4o-mini-search-preview-2025-03-11")
	assert.Contains(t, ids, "gpt-5.5-2026-04-23")
	assert.Contains(t, ids, "gpt-5.6-terra")
	assert.Contains(t, ids, "gpt-5.1-codex-max")
	assert.Contains(t, ids, "o4-mini-2025-04-16")
	assert.True(t, sort.StringsAreSorted(ids))

	original := append([]string(nil), ids...)
	ids[0] = "mutated"
	assert.Equal(t, original, ModelIDs())
}
