package anthropic

import (
	"fmt"
	"math"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
)

type reasoningConfig struct {
	thinking *ThinkingConfig
	effort   string
}

var effortMap = map[provider.ReasoningEffort]string{
	provider.ReasoningMinimal: "low",
	provider.ReasoningLow:     "low",
	provider.ReasoningMedium:  "medium",
	provider.ReasoningHigh:    "high",
	provider.ReasoningXHigh:   "max",
}

var budgetPercentages = map[provider.ReasoningEffort]float64{
	provider.ReasoningMinimal: 0.02,
	provider.ReasoningLow:     0.10,
	provider.ReasoningMedium:  0.30,
	provider.ReasoningHigh:    0.60,
	provider.ReasoningXHigh:   0.90,
}

const minReasoningBudget = 1024

func resolveReasoningConfig(reasoning provider.ReasoningEffort, caps modelCapabilities, warnings *[]provider.Warning) *reasoningConfig {
	if reasoning == provider.ReasoningNone {
		return &reasoningConfig{
			thinking: &ThinkingConfig{Type: ThinkingDisabled},
		}
	}

	if caps.supportsAdaptiveThinking {
		effort := mapReasoningToEffort(reasoning, caps.supportsXHighEffort, warnings)
		if effort == "" {
			return nil
		}
		return &reasoningConfig{
			thinking: &ThinkingConfig{Type: ThinkingAdaptive, Display: ThinkingDisplaySummarized},
			effort:   effort,
		}
	}

	budget := mapReasoningToBudget(reasoning, caps.maxOutputTokens, warnings)
	if budget < 0 {
		return nil
	}
	return &reasoningConfig{
		thinking: &ThinkingConfig{Type: ThinkingEnabled, BudgetTokens: budget},
	}
}

func mapReasoningToEffort(reasoning provider.ReasoningEffort, supportsXHighEffort bool, warnings *[]provider.Warning) string {
	mapped, ok := effortMap[reasoning]
	if !ok {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "reasoning",
			Details: fmt.Sprintf("reasoning %q is not supported by this model.", reasoning),
		})
		return ""
	}
	if reasoning == provider.ReasoningXHigh && supportsXHighEffort {
		mapped = string(provider.ReasoningXHigh)
	}

	if mapped != string(reasoning) {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnCompatibility,
			Feature: "reasoning",
			Details: fmt.Sprintf("reasoning %q is not directly supported by this model. mapped to effort %q.", reasoning, mapped),
		})
	}

	return mapped
}

func mapReasoningToBudget(reasoning provider.ReasoningEffort, maxOutputTokens int, warnings *[]provider.Warning) int {
	pct, ok := budgetPercentages[reasoning]
	if !ok {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "reasoning",
			Details: fmt.Sprintf("reasoning %q is not supported by this model.", reasoning),
		})
		return -1
	}

	budget := int(math.Round(float64(maxOutputTokens) * pct))
	if budget < minReasoningBudget {
		budget = minReasoningBudget
	}
	if budget > maxOutputTokens {
		budget = maxOutputTokens
	}
	return budget
}

// hasProviderEffort reports whether AnthropicOptions explicitly set Effort.
// Used to gate top-level Reasoning -> effort fallback so that provider-level
// effort takes precedence; thinking-only provider options still allow
// top-level reasoning to fill in the effort.
func hasProviderEffort(opts provider.ProviderOptions) bool {
	ao, ok, err := provider.ResolveOption[AnthropicOptions](opts, "anthropic")
	if err != nil || !ok {
		return false
	}
	return ao.Effort != ""
}

// providerThinkingType returns the ThinkingType set on AnthropicOptions, or
// the empty string if unset. Used to honor upstream's "do not derive effort
// when provider thinking is disabled" rule (anthropic-language-model.ts:408).
func providerThinkingType(opts provider.ProviderOptions) ThinkingType {
	ao, ok, err := provider.ResolveOption[AnthropicOptions](opts, "anthropic")
	if err != nil || !ok || ao.Thinking == nil {
		return ""
	}
	return ao.Thinking.Type
}

// applyReasoningConfigWithProviderHints applies a reasoning-derived config to
// p, honoring the rule that effort is not derived when provider options
// already set thinking to "disabled" (mirrors upstream
// anthropic-language-model.ts:406-411). Thinking is only applied when the
// provider has not set thinking at all.
func applyReasoningConfigWithProviderHints(p *anthropic.BetaMessageNewParams, rc *reasoningConfig, providerThinking ThinkingType) {
	if rc.thinking != nil && providerThinking == "" {
		switch rc.thinking.Type {
		case ThinkingEnabled:
			p.Thinking = anthropic.BetaThinkingConfigParamUnion{
				OfEnabled: &anthropic.BetaThinkingConfigEnabledParam{
					BudgetTokens: int64(rc.thinking.BudgetTokens),
				},
			}
		case ThinkingDisabled:
			p.Thinking = anthropic.BetaThinkingConfigParamUnion{
				OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{},
			}
		case ThinkingAdaptive:
			p.Thinking = anthropic.BetaThinkingConfigParamUnion{
				OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{
					Display: anthropic.BetaThinkingConfigAdaptiveDisplay(rc.thinking.Display),
				},
			}
		}
	}
	if rc.effort != "" && providerThinking != ThinkingDisabled {
		p.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(rc.effort)
	}
}
