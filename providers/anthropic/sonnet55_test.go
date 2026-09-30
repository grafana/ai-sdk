package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Sonnet 5.5 behavior mirrors @ai-sdk/anthropic 4.0.67: thinking cannot be
// disabled (between_tools is the lowest setting), forced tool use is rejected,
// and sampling parameters are rejected.

func TestSonnet55ModelIDs(t *testing.T) {
	assert.Contains(t, ModelIDs(), "claude-sonnet-5-5")
	assert.Contains(t, VertexModelIDs(), "claude-sonnet-5-5")
	assert.Contains(t, DualAvailableModelIDs(), "claude-sonnet-5-5")
	assert.Equal(t, "claude-sonnet-5-5", ResolveVertexModelID("claude-sonnet-5-5"), "Google Cloud serves the undated ID")
}

func TestSonnet55Capabilities(t *testing.T) {
	caps := getModelCapabilities("claude-sonnet-5-5")
	assert.Equal(t, 128000, caps.maxOutputTokens)
	assert.True(t, caps.supportsAdaptiveThinking)
	assert.True(t, caps.supportsStructuredOutput)
	assert.True(t, caps.rejectsSamplingParams)
	assert.True(t, caps.supportsXHighEffort)
	assert.True(t, caps.rejectsThinkingDisabled)
	assert.True(t, caps.rejectsForcedToolUse)
	assert.True(t, caps.supportsBetweenToolsThinking)
	assert.True(t, caps.isKnownModel)

	for _, id := range []string{"anthropic.claude-sonnet-5-5", "global.anthropic.claude-sonnet-5-5"} {
		assert.True(t, getModelCapabilities(id).supportsBetweenToolsThinking, id)
	}

	sonnet5 := getModelCapabilities("claude-sonnet-5")
	assert.False(t, sonnet5.rejectsThinkingDisabled)
	assert.False(t, sonnet5.rejectsForcedToolUse)
	assert.False(t, sonnet5.supportsBetweenToolsThinking)
}

func TestSonnet55ReasoningNoneUsesBetweenTools(t *testing.T) {
	p, _, warnings, _, err := buildParams("claude-sonnet-5-5", provider.CallOptions{Reasoning: provider.ReasoningNone}, false)
	require.NoError(t, err)
	assertThinkingJSON(t, p.Thinking, `{"type":"between_tools"}`)
	assert.Empty(t, string(p.OutputConfig.Effort))
	assert.Empty(t, warnings)

	// Sonnet 5 still disables thinking.
	p, _, _, _, err = buildParams("claude-sonnet-5", provider.CallOptions{Reasoning: provider.ReasoningNone}, false)
	require.NoError(t, err)
	require.NotNil(t, p.Thinking.OfDisabled)
}

func TestSonnet55ReasoningEffortUsesAdaptive(t *testing.T) {
	p, _, warnings, _, err := buildParams("claude-sonnet-5-5", provider.CallOptions{Reasoning: provider.ReasoningHigh}, false)
	require.NoError(t, err)
	require.NotNil(t, p.Thinking.OfAdaptive)
	assert.Equal(t, "summarized", string(p.Thinking.OfAdaptive.Display))
	assert.Equal(t, "high", string(p.OutputConfig.Effort))
	assert.Empty(t, warnings)
}

func TestSonnet55ExplicitThinkingIsNormalized(t *testing.T) {
	tests := []struct {
		name         string
		thinking     ThinkingConfig
		effort       string
		wantThinking string
		wantEffort   string
		wantFeatures []string
	}{
		{
			name:         "disabled becomes between_tools",
			thinking:     ThinkingConfig{Type: ThinkingDisabled},
			wantThinking: `{"type":"between_tools"}`,
			wantFeatures: []string{"providerOptions.anthropic.thinking"},
		},
		{
			name:         "disabled at max effort becomes between_tools at high",
			thinking:     ThinkingConfig{Type: ThinkingDisabled},
			effort:       "max",
			wantThinking: `{"type":"between_tools"}`,
			wantEffort:   "high",
			wantFeatures: []string{"providerOptions.anthropic.thinking", "providerOptions.anthropic.effort"},
		},
		{
			name:         "between_tools at xhigh is lowered to high",
			thinking:     ThinkingConfig{Type: ThinkingBetweenTools},
			effort:       "xhigh",
			wantThinking: `{"type":"between_tools"}`,
			wantEffort:   "high",
			wantFeatures: []string{"providerOptions.anthropic.effort"},
		},
		{
			name:         "between_tools drops display",
			thinking:     ThinkingConfig{Type: ThinkingBetweenTools, Display: ThinkingDisplaySummarized},
			effort:       "medium",
			wantThinking: `{"type":"between_tools"}`,
			wantEffort:   "medium",
		},
		{
			name:         "budget thinking becomes adaptive",
			thinking:     ThinkingConfig{Type: ThinkingEnabled, BudgetTokens: 4096},
			wantThinking: `{"type":"adaptive"}`,
			wantFeatures: []string{"providerOptions.anthropic.thinking"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			thinking := tc.thinking
			opts := provider.CallOptions{ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{
				Thinking: &thinking,
				Effort:   tc.effort,
			})}
			p, _, warnings, _, err := buildParams("claude-sonnet-5-5", opts, false)
			require.NoError(t, err)
			assertThinkingJSON(t, p.Thinking, tc.wantThinking)
			assert.Equal(t, tc.wantEffort, string(p.OutputConfig.Effort))
			assert.ElementsMatch(t, tc.wantFeatures, warningFeatures(warnings))
		})
	}
}

func TestSonnet55DropsSamplingParameters(t *testing.T) {
	temperature := 0.2
	p, _, warnings, _, err := buildParams("claude-sonnet-5-5", provider.CallOptions{Reasoning: provider.ReasoningNone, Temperature: &temperature}, false)
	require.NoError(t, err)
	assert.False(t, p.Temperature.Valid())
	assert.Contains(t, warningFeatures(warnings), "temperature")
}

func TestSonnet55ForcedToolChoiceFallsBackToAuto(t *testing.T) {
	tools := []provider.Tool{
		{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Type: provider.ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}

	t.Run("required", func(t *testing.T) {
		p, _, warnings, _, err := buildParams("claude-sonnet-5-5", provider.CallOptions{
			Tools:      tools,
			ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceRequired},
		}, false)
		require.NoError(t, err)
		require.NotNil(t, p.ToolChoice.OfAuto)
		assert.Nil(t, p.ToolChoice.OfAny)
		assert.Len(t, p.Tools, 2)
		assert.Contains(t, warningFeatures(warnings), "toolChoice")
	})

	t.Run("named tool", func(t *testing.T) {
		p, _, warnings, _, err := buildParams("claude-sonnet-5-5", provider.CallOptions{
			Tools:      tools,
			ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "search"},
		}, false)
		require.NoError(t, err)
		require.NotNil(t, p.ToolChoice.OfAuto)
		assert.Nil(t, p.ToolChoice.OfTool)
		require.Len(t, p.Tools, 1)
		assert.Equal(t, "search", *p.Tools[0].GetName())
		assert.Contains(t, warningFeatures(warnings), "toolChoice")
	})

	t.Run("sonnet 5 keeps forced tool use", func(t *testing.T) {
		p, _, _, _, err := buildParams("claude-sonnet-5", provider.CallOptions{
			Tools:      tools,
			ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceRequired},
		}, false)
		require.NoError(t, err)
		require.NotNil(t, p.ToolChoice.OfAny)
	})
}

func TestSonnet55JSONToolModeUsesOutputFormat(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	opts := provider.CallOptions{
		ResponseFormat:  &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
		ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{StructuredOutputMode: StructuredOutputJSONTool}),
	}
	for name, caps := range map[string]providerCapabilities{
		"direct": directProviderCapabilities,
		"vertex": vertexProviderCapabilities,
	} {
		t.Run(name, func(t *testing.T) {
			p, _, warnings, br, err := buildParamsWithCapabilities("claude-sonnet-5-5", opts, false, caps)
			require.NoError(t, err)
			assert.False(t, br.usesJsonResponseTool)
			assert.NotEmpty(t, p.OutputConfig.Format.Schema)
			assert.Nil(t, p.ToolChoice.OfAny)
			assert.Contains(t, warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "providerOptions.anthropic.structuredOutputMode",
				Details: "structuredOutputMode 'jsonTool' is not supported by claude-sonnet-5-5 because it rejects forced tool use. Using 'outputFormat' instead.",
			})
		})
	}
}

func TestSonnet55JSONToolWithoutNativeOutputUsesAuto(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	p, _, warnings, br, err := buildParamsWithCapabilities("claude-sonnet-5-5", provider.CallOptions{
		ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
	}, false, providerCapabilities{})
	require.NoError(t, err)
	assert.True(t, br.usesJsonResponseTool)
	require.NotNil(t, p.ToolChoice.OfAuto)
	assert.Nil(t, p.ToolChoice.OfAny)
	assert.Contains(t, warningFeatures(warnings), "toolChoice")

	// The JSON response tool replaces the caller's tool choice, as upstream
	// does, so a forced caller choice keeps every tool and warns once.
	tools := []provider.Tool{
		{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Type: provider.ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	for _, tc := range []provider.ToolChoice{
		{Type: provider.ToolChoiceRequired},
		{Type: provider.ToolChoiceTool, ToolName: "search"},
	} {
		t.Run(string(tc.Type), func(t *testing.T) {
			p, _, warnings, br, err := buildParamsWithCapabilities("claude-sonnet-5-5", provider.CallOptions{
				Tools:          tools,
				ToolChoice:     &tc,
				ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
			}, false, providerCapabilities{})
			require.NoError(t, err)
			assert.True(t, br.usesJsonResponseTool)
			require.NotNil(t, p.ToolChoice.OfAuto)
			assert.True(t, p.ToolChoice.OfAuto.DisableParallelToolUse.Value)
			var names []string
			for _, tool := range p.Tools {
				names = append(names, *tool.GetName())
			}
			assert.Equal(t, []string{"weather", "search", "json"}, names)
			var toolChoiceWarnings []provider.Warning
			for _, w := range warnings {
				if w.Feature == "toolChoice" {
					toolChoiceWarnings = append(toolChoiceWarnings, w)
				}
			}
			require.Len(t, toolChoiceWarnings, 1)
			assert.Equal(t, forcedToolChoiceWarning(provider.ToolChoice{Type: provider.ToolChoiceRequired}), toolChoiceWarnings[0])
		})
	}
}

func assertThinkingJSON(t *testing.T, thinking any, want string) {
	t.Helper()
	got, err := json.Marshal(thinking)
	require.NoError(t, err)
	assert.JSONEq(t, want, string(got))
}
