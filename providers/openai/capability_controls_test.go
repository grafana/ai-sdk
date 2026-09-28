package openai

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_GPT6ReasoningEffortCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name     string
		model    string
		effort   string
		fallback provider.ReasoningEffort
		want     string
		warn     bool
		force    bool
	}{
		{name: "supported option", model: "gpt-6-astra", effort: "max", want: "max"},
		{name: "unsupported option", model: "gpt-6-astra", effort: "none", warn: true},
		{name: "unsupported fallback", model: "gpt-7", fallback: provider.ReasoningMinimal, warn: true},
		{name: "supported fallback", model: "gpt-6", fallback: provider.ReasoningHigh, want: "high"},
		{name: "option precedes fallback", model: "gpt-6", effort: "minimal", fallback: provider.ReasoningHigh, warn: true},
		{name: "earlier model remains unchanged", model: "gpt-5.6", effort: "none", want: "none"},
		{name: "unrecognized ID forced reasoning", model: "gpt-6chat", effort: "minimal", want: "minimal", force: true},
		{name: "mantle override", model: "openai.gpt-6-astra", effort: "none", want: "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := OpenAIResponsesOptions{ReasoningEffort: tc.effort}
			if tc.force {
				force := true
				options.ForceReasoning = &force
			}
			body, warnings := buildBody(t, tc.model, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: tc.fallback,
				ProviderOptions: withOpenAIOptions(options),
			})
			assert.Equal(t, tc.effort, options.ReasoningEffort)
			if tc.want == "" {
				if reasoning, ok := body["reasoning"].(map[string]any); ok {
					assert.NotContains(t, reasoning, "effort")
				}
			} else {
				reasoning, ok := body["reasoning"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tc.want, reasoning["effort"])
			}
			if tc.warn {
				require.Contains(t, warningFeatures(warnings), "reasoningEffort")
				for _, warning := range warnings {
					if warning.Feature == "reasoningEffort" {
						assert.Equal(t, provider.WarnUnsupported, warning.Type)
						assert.Contains(t, warning.Details, "low, medium, high, xhigh, max")
						break
					}
				}
			} else {
				assert.NotContains(t, warningFeatures(warnings), "reasoningEffort")
			}
		})
	}
}
