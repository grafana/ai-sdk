package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelCapabilities(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    modelCapabilities
	}{
		{"opus 5", "claude-opus-5", modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsStructuredOutput: true, rejectsSamplingParams: true, supportsXHighEffort: true, rejectsThinkingDisabledAboveHighEffort: true, isKnownModel: true}},
		{"opus 4-7", "claude-opus-4-7", modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsStructuredOutput: true, rejectsSamplingParams: true, supportsXHighEffort: true, isKnownModel: true}},
		{"sonnet 5", "claude-sonnet-5", modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsStructuredOutput: true, rejectsSamplingParams: true, supportsXHighEffort: true, isKnownModel: true}},
		{"sonnet 4-6", "claude-sonnet-4-6-20260101", modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsStructuredOutput: true, isKnownModel: true}},
		{"opus 4-6", "claude-opus-4-6-20260101", modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsStructuredOutput: true, isKnownModel: true}},
		{"sonnet 4-5", "claude-sonnet-4-5-20250514", modelCapabilities{maxOutputTokens: 64000, supportsStructuredOutput: true, isKnownModel: true}},
		{"opus 4-5", "claude-opus-4-5-20250514", modelCapabilities{maxOutputTokens: 64000, supportsStructuredOutput: true, isKnownModel: true}},
		{"haiku 4-5", "claude-haiku-4-5-20250514", modelCapabilities{maxOutputTokens: 64000, supportsStructuredOutput: true, isKnownModel: true}},
		{"opus 4-1", "claude-opus-4-1-20250414", modelCapabilities{maxOutputTokens: 32000, supportsStructuredOutput: true, isKnownModel: true}},
		{"generic sonnet 4-", "claude-sonnet-4-3-20250101", modelCapabilities{maxOutputTokens: 64000, isKnownModel: true}},
		{"generic opus 4-", "claude-opus-4-2-20250101", modelCapabilities{maxOutputTokens: 32000, isKnownModel: true}},
		{"claude 3 haiku", "claude-3-haiku-20240307", modelCapabilities{maxOutputTokens: 4096, isKnownModel: true}},
		{"legacy claude 3.7", "us.anthropic.claude-3-7-sonnet-20250219-v1:0", modelCapabilities{maxOutputTokens: 4096}},
		{"legacy claude 2", "anthropic.claude-v2:1", modelCapabilities{maxOutputTokens: 4096}},
		{"future claude", "claude-future-9", modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsStructuredOutput: true, rejectsSamplingParams: true, supportsXHighEffort: true, rejectsThinkingDisabledAboveHighEffort: true}},
		{"platform future claude", "us.anthropic.claude-future-9-20990101-v1:0", modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsStructuredOutput: true, rejectsSamplingParams: true, supportsXHighEffort: true, rejectsThinkingDisabledAboveHighEffort: true}},
		{"unknown model", "some-future-model", modelCapabilities{maxOutputTokens: 4096}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getModelCapabilities(tc.modelID)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveReasoningConfig_XHighEffort(t *testing.T) {
	caps := modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, supportsXHighEffort: true}
	var warnings []provider.Warning

	rc := resolveReasoningConfig(provider.ReasoningXHigh, caps, &warnings)
	require.NotNil(t, rc)
	assert.Equal(t, ThinkingAdaptive, rc.thinking.Type)
	assert.Equal(t, ThinkingDisplaySummarized, rc.thinking.Display)
	assert.Equal(t, "xhigh", rc.effort)
	assert.Empty(t, warnings)
}

func TestResolveReasoningConfig_AdaptivePath(t *testing.T) {
	caps := modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true, isKnownModel: true}

	tests := []struct {
		name            string
		reasoning       provider.ReasoningEffort
		wantThinking    ThinkingType
		wantEffort      string
		wantWarningType provider.WarningType
	}{
		{"minimal maps to low with warning", provider.ReasoningMinimal, ThinkingAdaptive, "low", provider.WarnCompatibility},
		{"low maps to low", provider.ReasoningLow, ThinkingAdaptive, "low", ""},
		{"medium maps to medium", provider.ReasoningMedium, ThinkingAdaptive, "medium", ""},
		{"high maps to high", provider.ReasoningHigh, ThinkingAdaptive, "high", ""},
		{"xhigh maps to max with warning", provider.ReasoningXHigh, ThinkingAdaptive, "max", provider.WarnCompatibility},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var warnings []provider.Warning
			rc := resolveReasoningConfig(tc.reasoning, caps, &warnings)
			require.NotNil(t, rc)
			assert.Equal(t, tc.wantThinking, rc.thinking.Type)
			assert.Equal(t, ThinkingDisplaySummarized, rc.thinking.Display)
			assert.Equal(t, tc.wantEffort, rc.effort)

			if tc.wantWarningType != "" {
				require.Len(t, warnings, 1)
				assert.Equal(t, tc.wantWarningType, warnings[0].Type)
				assert.Equal(t, "reasoning", warnings[0].Feature)
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}

func TestResolveReasoningConfig_BudgetPath(t *testing.T) {
	tests := []struct {
		name       string
		reasoning  provider.ReasoningEffort
		caps       modelCapabilities
		wantBudget int
	}{
		{"medium on sonnet 4-5", provider.ReasoningMedium, modelCapabilities{maxOutputTokens: 64000}, 19200},
		{"minimal on sonnet 4-5", provider.ReasoningMinimal, modelCapabilities{maxOutputTokens: 64000}, 1280},
		{"xhigh on sonnet 4-5", provider.ReasoningXHigh, modelCapabilities{maxOutputTokens: 64000}, 57600},
		{"low on sonnet 4-5", provider.ReasoningLow, modelCapabilities{maxOutputTokens: 64000}, 6400},
		{"high on sonnet 4-5", provider.ReasoningHigh, modelCapabilities{maxOutputTokens: 64000}, 38400},
		{"minimal clamped to 1024", provider.ReasoningMinimal, modelCapabilities{maxOutputTokens: 4096}, 1024},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var warnings []provider.Warning
			rc := resolveReasoningConfig(tc.reasoning, tc.caps, &warnings)
			require.NotNil(t, rc)
			assert.Equal(t, ThinkingEnabled, rc.thinking.Type)
			assert.Equal(t, tc.wantBudget, rc.thinking.BudgetTokens)
			assert.Empty(t, rc.effort)
			assert.Empty(t, warnings)
		})
	}
}

func TestResolveReasoningConfig_None(t *testing.T) {
	caps := modelCapabilities{maxOutputTokens: 128000, supportsAdaptiveThinking: true}
	var warnings []provider.Warning
	rc := resolveReasoningConfig(provider.ReasoningNone, caps, &warnings)
	require.NotNil(t, rc)
	assert.Equal(t, ThinkingDisabled, rc.thinking.Type)
	assert.Empty(t, rc.effort)
	assert.Empty(t, warnings)
}

func TestBuildParams_ReasoningPrecedence(t *testing.T) {
	t.Run("provider thinking set skips reasoning", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		opts := provider.CallOptions{
			Reasoning: reasoning,
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":5000}}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.NotNil(t, p.Thinking.OfEnabled)
		assert.Equal(t, int64(5000), p.Thinking.OfEnabled.BudgetTokens)
	})

	t.Run("provider effort set skips reasoning", func(t *testing.T) {
		reasoning := provider.ReasoningLow
		opts := provider.CallOptions{
			Reasoning: reasoning,
			ProviderOptions: provider.ProviderOptions{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"effort":"max"}`)},
			},
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.Equal(t, "max", string(p.OutputConfig.Effort))
		assert.Nil(t, p.Thinking.OfAdaptive, "reasoning should not set adaptive thinking")
	})

	t.Run("neither set allows reasoning mapping", func(t *testing.T) {
		reasoning := provider.ReasoningMedium
		opts := provider.CallOptions{
			Reasoning: reasoning,
		}

		p, _, warnings, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		require.NotNil(t, p.Thinking.OfAdaptive, "should set adaptive thinking from reasoning")
		assert.Equal(t, "summarized", string(p.Thinking.OfAdaptive.Display))
		assert.Equal(t, "medium", string(p.OutputConfig.Effort))
		assert.Empty(t, warnings)
	})
}

func TestBuildParams_Opus47SupportsXHighEffort(t *testing.T) {
	reasoning := provider.ReasoningXHigh
	opts := provider.CallOptions{
		Reasoning: reasoning,
	}

	p, _, warnings, _, err := buildParams("claude-opus-4-7", opts, false)
	require.NoError(t, err)

	require.NotNil(t, p.Thinking.OfAdaptive)
	assert.Equal(t, "summarized", string(p.Thinking.OfAdaptive.Display))
	assert.Equal(t, "xhigh", string(p.OutputConfig.Effort))
	assert.NotContains(t, p.Betas, "effort-2025-11-24",
		"effort-2025-11-24 beta is GA and rejected by Vertex AI; must not be appended")
	assert.Empty(t, warnings)
}

func TestBuildParams_ThinkingDisabledEffortConstraint(t *testing.T) {
	tests := []struct {
		name        string
		modelID     string
		effort      string
		wantEffort  string
		wantWarning bool
	}{
		{name: "opus 5 xhigh", modelID: "claude-opus-5", effort: "xhigh", wantEffort: "high", wantWarning: true},
		{name: "opus 5 max", modelID: "claude-opus-5", effort: "max", wantEffort: "high", wantWarning: true},
		{name: "future Claude xhigh", modelID: "claude-future-9", effort: "xhigh", wantEffort: "high", wantWarning: true},
		{name: "opus 5 high", modelID: "claude-opus-5", effort: "high", wantEffort: "high"},
		{name: "opus 4.8 unchanged", modelID: "claude-opus-4-8", effort: "xhigh", wantEffort: "xhigh"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := provider.CallOptions{ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{
				Thinking: &ThinkingConfig{Type: ThinkingDisabled},
				Effort:   tc.effort,
			})}

			p, _, warnings, _, err := buildParams(tc.modelID, opts, false)
			require.NoError(t, err)
			require.NotNil(t, p.Thinking.OfDisabled)
			assert.Equal(t, tc.wantEffort, string(p.OutputConfig.Effort))
			if tc.wantWarning {
				var effortWarning *provider.Warning
				for i := range warnings {
					if warnings[i].Feature == "providerOptions.anthropic.effort" {
						effortWarning = &warnings[i]
					}
				}
				require.NotNil(t, effortWarning)
				assert.Equal(t, provider.WarnUnsupported, effortWarning.Type)
			} else {
				assert.NotContains(t, warningFeatures(warnings), "providerOptions.anthropic.effort")
			}
		})
	}
}

func TestBuildParams_ReasoningProviderDefault(t *testing.T) {
	t.Run("zero-valued reasoning is no-op", func(t *testing.T) {
		opts := provider.CallOptions{}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.Nil(t, p.Thinking.OfEnabled)
		assert.Nil(t, p.Thinking.OfAdaptive)
		assert.Nil(t, p.Thinking.OfDisabled)
		assert.Empty(t, p.OutputConfig.Effort)
	})

	t.Run("provider-default reasoning is no-op", func(t *testing.T) {
		reasoning := provider.ReasoningProviderDefault
		opts := provider.CallOptions{
			Reasoning: reasoning,
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.Nil(t, p.Thinking.OfEnabled)
		assert.Nil(t, p.Thinking.OfAdaptive)
		assert.Nil(t, p.Thinking.OfDisabled)
		assert.Empty(t, p.OutputConfig.Effort)
	})
}

func TestBuildParams_ReasoningBetaHeaders(t *testing.T) {
	t.Run("adaptive path gets no beta headers", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		opts := provider.CallOptions{
			Reasoning: reasoning,
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.NotContains(t, p.Betas, "effort-2025-11-24",
			"effort-2025-11-24 beta is GA and rejected by Vertex AI; must not be appended")
		assert.NotContains(t, p.Betas, "interleaved-thinking-2025-05-14",
			"interleaved-thinking beta is no longer sent by upstream")
	})

	t.Run("budget path gets no beta headers", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		opts := provider.CallOptions{
			Reasoning: reasoning,
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-5-20250514", opts, false)
		require.NoError(t, err)

		assert.NotContains(t, p.Betas, "effort-2025-11-24",
			"budget path must not include effort beta")
		assert.NotContains(t, p.Betas, "interleaved-thinking-2025-05-14",
			"budget path must not include interleaved thinking beta")
	})

	t.Run("none gets no thinking or effort betas", func(t *testing.T) {
		reasoning := provider.ReasoningNone
		opts := provider.CallOptions{
			Reasoning: reasoning,
		}

		p, _, _, _, err := buildParams("claude-sonnet-4-6", opts, false)
		require.NoError(t, err)

		assert.NotContains(t, p.Betas, "interleaved-thinking-2025-05-14",
			"none must not add thinking beta")
		assert.NotContains(t, p.Betas, "effort-2025-11-24",
			"none must not add effort beta")
	})
}
