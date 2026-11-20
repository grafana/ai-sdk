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
		{"gpt-6-astra", true, "developer", true, true, false},
		{"openai.gpt-6-astra", true, "developer", false, false, false},
		{"gpt-6-sol", true, "developer", true, true, true},
		{"gpt-6-luna", true, "developer", true, true, true},
		{"gpt-6.1-sol", true, "developer", true, true, false},
		{"gpt-6.1-terra", true, "developer", true, true, true},
		{"gpt-6.1-terra-fast", true, "developer", true, true, false},
		{"openai.gpt-6.1-terra", true, "developer", false, false, false},
		{"gpt-6-sol-fast", true, "developer", true, true, false},
		{"gpt-6-luna-2026-09-01", true, "developer", true, true, false},
		{"openai.gpt-6-sol", true, "developer", false, false, false},
		{"openai.gpt-6-luna", true, "developer", false, false, false},
		{"gpt-5.6", true, "developer", true, true, true},
		{"gpt-5.6-luna", true, "developer", true, true, true},
		{"openai.gpt-5.6-luna", true, "developer", false, false, false},
		{"gpt-5.99-chat-latest", true, "developer", true, true, true},
		{"gpt-99", true, "developer", true, true, false},
		{"gpt-99-mini", true, "developer", true, true, false},
		{"gpt-99-nano", true, "developer", true, false, false},
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

func TestGetModelCapabilities_GPT6Controls(t *testing.T) {
	positiveEfforts := []string{"low", "medium", "high", "xhigh", "max"}
	nonReasoningEfforts := []string{"none", "low", "medium", "high", "xhigh", "max"}
	for _, tc := range []struct {
		modelID     string
		supported   bool
		wantEfforts []string
	}{
		{modelID: "gpt-6-astra", supported: true, wantEfforts: positiveEfforts},
		{modelID: "gpt-6-sol", supported: true, wantEfforts: nonReasoningEfforts},
		{modelID: "gpt-6-luna", supported: true, wantEfforts: nonReasoningEfforts},
		{modelID: "gpt-6.1-terra", supported: true, wantEfforts: nonReasoningEfforts},
		{modelID: "gpt-6.1-terra-fast", supported: true, wantEfforts: positiveEfforts},
		{modelID: "openai.gpt-6.1-terra"},
		{modelID: "gpt-6.1-sol", supported: true, wantEfforts: positiveEfforts},
		{modelID: "gpt-6-sol-fast", supported: true, wantEfforts: positiveEfforts},
		{modelID: "gpt-7", supported: true, wantEfforts: positiveEfforts},
		{modelID: "gpt-99-nano", supported: true, wantEfforts: positiveEfforts},
		{modelID: "gpt-5.6", wantEfforts: nonReasoningEfforts},
		{modelID: "gpt-6chat"},
		{modelID: "openai.gpt-6-astra"},
		{modelID: "openai.gpt-6-sol"},
		{modelID: "ft:gpt-6:org:custom:abc"},
	} {
		t.Run(tc.modelID, func(t *testing.T) {
			caps := getModelCapabilities(tc.modelID)
			assert.Equal(t, tc.supported, caps.supportsConfigurationUpdate)
			assert.Equal(t, tc.supported, caps.supportsAsyncToolCalling)
			assert.Equal(t, tc.wantEfforts, caps.supportedReasoningEfforts)
		})
	}
}

func TestGetModelCapabilities_GPT5Efforts(t *testing.T) {
	for _, tc := range []struct {
		modelID string
		efforts []string
	}{
		{modelID: "gpt-5-nano-2025-08-07", efforts: []string{"minimal", "low", "medium", "high"}},
		{modelID: "gpt-5.1-2025-11-13", efforts: []string{"none", "low", "medium", "high"}},
		{modelID: "gpt-5.2", efforts: []string{"none", "low", "medium", "high", "xhigh"}},
		{modelID: "gpt-5.4-mini-2026-03-17", efforts: []string{"none", "low", "medium", "high", "xhigh"}},
		{modelID: "gpt-5.5", efforts: []string{"none", "low", "medium", "high", "xhigh"}},
		{modelID: "gpt-5.6-luna", efforts: []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{modelID: "gpt-5.7-terra-2027-01-01", efforts: []string{"none", "low", "medium", "high", "xhigh", "max"}},
		{modelID: "gpt-5-pro"},
		{modelID: "gpt-5-codex"},
		{modelID: "gpt-5-chat-latest"},
		{modelID: "gpt-5.4-mini-fast"},
		{modelID: "openai.gpt-5.6-luna"},
	} {
		t.Run(tc.modelID, func(t *testing.T) {
			assert.Equal(t, tc.efforts, getModelCapabilities(tc.modelID).supportedReasoningEfforts)
		})
	}
}

func TestModelIDs(t *testing.T) {
	ids := ModelIDs()
	assert.Len(t, ids, 72)
	assert.Contains(t, ids, "gpt-4o")
	assert.Contains(t, ids, "gpt-4o-2024-11-20")
	assert.Contains(t, ids, "gpt-4o-audio-preview")
	assert.Contains(t, ids, "gpt-4o-mini-search-preview-2025-03-11")
	assert.Contains(t, ids, "gpt-5.5-2026-04-23")
	assert.Contains(t, ids, "gpt-5.6-terra")
	assert.Contains(t, ids, "gpt-6-astra")
	assert.Contains(t, ids, "gpt-6-luna")
	assert.Contains(t, ids, "gpt-6-sol")
	assert.Contains(t, ids, "gpt-6.1-sol")
	assert.Contains(t, ids, "gpt-6.1-terra")
	assert.Contains(t, ids, "gpt-5.1-codex-max")
	assert.Contains(t, ids, "o4-mini-2025-04-16")
	assert.True(t, sort.StringsAreSorted(ids))

	original := append([]string(nil), ids...)
	ids[0] = "mutated"
	assert.Equal(t, original, ModelIDs())
}
