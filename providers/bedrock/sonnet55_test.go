package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSonnet55Model = "global.anthropic.claude-sonnet-5-5"

func TestSonnet55ModelID(t *testing.T) {
	assert.Contains(t, ModelIDs(), "anthropic.claude-sonnet-5-5")
}

func TestSonnet55ReasoningNoneUsesBetweenTools(t *testing.T) {
	req, warnings, _ := mustBuildRequest(t, testSonnet55Model, provider.CallOptions{
		Prompt:    []provider.Message{provider.UserText("x")},
		Reasoning: provider.ReasoningNone,
	})
	assert.Empty(t, warnings)
	assert.Equal(t, map[string]any{"type": "between_tools"}, req.AdditionalModelRequestFields["thinking"])
	assert.NotContains(t, req.AdditionalModelRequestFields, "output_config")

	// Sonnet 5 keeps the upstream behavior: disabled omits the thinking field.
	req, _, _ = mustBuildRequest(t, "global.anthropic.claude-sonnet-5", provider.CallOptions{
		Prompt:    []provider.Message{provider.UserText("x")},
		Reasoning: provider.ReasoningNone,
	})
	assert.NotContains(t, req.AdditionalModelRequestFields, "thinking")
}

func TestSonnet55ReasoningEffortUsesAdaptive(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testSonnet55Model, provider.CallOptions{
		Prompt:    []provider.Message{provider.UserText("x")},
		Reasoning: provider.ReasoningHigh,
	})
	assert.Equal(t, map[string]any{"type": "adaptive"}, req.AdditionalModelRequestFields["thinking"])
	assert.Equal(t, map[string]any{"effort": "high"}, req.AdditionalModelRequestFields["output_config"])
}

func TestSonnet55BetweenToolsEffortIsCapped(t *testing.T) {
	req, warnings, _ := mustBuildRequest(t, testSonnet55Model, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("x")},
		ProviderOptions: provider.BuildProviderOptions(BedrockOptions{
			ReasoningConfig: &ReasoningConfig{Type: "between_tools", MaxReasoningEffort: "max"},
		}),
	})
	assert.Equal(t, map[string]any{"type": "between_tools"}, req.AdditionalModelRequestFields["thinking"])
	assert.Equal(t, map[string]any{"effort": "high"}, req.AdditionalModelRequestFields["output_config"])
	require.Len(t, warnings, 1)
	assert.Equal(t, "providerOptions.amazonBedrock.reasoningConfig.maxReasoningEffort", warnings[0].Feature)
}

func TestSonnet55ForcedToolChoiceFallsBackToAuto(t *testing.T) {
	tools := []provider.Tool{
		{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Type: provider.ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	t.Run("required", func(t *testing.T) {
		req, warnings, _ := mustBuildRequest(t, testSonnet55Model, provider.CallOptions{
			Prompt:     []provider.Message{provider.UserText("x")},
			Tools:      tools,
			ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceRequired},
		})
		require.NotNil(t, req.ToolConfig)
		assert.Len(t, req.ToolConfig.Tools, 2)
		assert.Equal(t, &toolChoiceUnion{Auto: &struct{}{}}, req.ToolConfig.ToolChoice)
		assert.Equal(t, "toolChoice", warnings[0].Feature)
	})

	t.Run("named tool", func(t *testing.T) {
		req, warnings, _ := mustBuildRequest(t, testSonnet55Model, provider.CallOptions{
			Prompt:     []provider.Message{provider.UserText("x")},
			Tools:      tools,
			ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "search"},
		})
		require.NotNil(t, req.ToolConfig)
		require.Len(t, req.ToolConfig.Tools, 1)
		assert.Equal(t, "search", req.ToolConfig.Tools[0].ToolSpec.Name)
		assert.Equal(t, &toolChoiceUnion{Auto: &struct{}{}}, req.ToolConfig.ToolChoice)
		assert.Equal(t, "toolChoice", warnings[0].Feature)
	})

	t.Run("sonnet 5 keeps forced tool use", func(t *testing.T) {
		req, _, _ := mustBuildRequest(t, "global.anthropic.claude-sonnet-5", provider.CallOptions{
			Prompt:     []provider.Message{provider.UserText("x")},
			Tools:      tools,
			ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceRequired},
		})
		assert.Equal(t, &toolChoiceUnion{Any: &struct{}{}}, req.ToolConfig.ToolChoice)
	})
}

func TestSonnet55JSONResponseUsesInstructionWithoutTools(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	for _, mode := range []StructuredOutputMode{StructuredOutputModeAuto, StructuredOutputModeJSONTool} {
		t.Run(string(mode), func(t *testing.T) {
			req, _, meta := mustBuildRequest(t, testSonnet55Model, provider.CallOptions{
				Prompt:          []provider.Message{provider.UserText("x")},
				ResponseFormat:  &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
				ProviderOptions: provider.BuildProviderOptions(BedrockOptions{StructuredOutputMode: mode}),
			})
			assert.True(t, meta.usesJSONInstruction)
			assert.False(t, meta.usesJSONResponseTool)
			assert.Nil(t, req.ToolConfig, "no forced JSON tool")
		})
	}
}
