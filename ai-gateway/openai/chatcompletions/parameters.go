package chatcompletions

import (
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

type chatParameters struct {
	User                 *string            `json:"user"`
	SafetyIdentifier     *string            `json:"safety_identifier"`
	ServiceTier          *string            `json:"service_tier"`
	PromptCacheKey       *string            `json:"prompt_cache_key"`
	PromptCacheRetention *string            `json:"prompt_cache_retention"`
	Metadata             map[string]string  `json:"metadata"`
	LogitBias            map[string]float64 `json:"logit_bias"`
	Logprobs             *bool              `json:"logprobs"`
	TopLogprobs          *int               `json:"top_logprobs"`
}

func (r mappedRequest) translateParameters(target execution.Target, values map[string]any) error {
	fields := []struct {
		chat, native string
		value        *string
	}{
		{"user", "user", r.User},
		{"safety_identifier", "safetyIdentifier", r.SafetyIdentifier},
		{"service_tier", "serviceTier", r.ServiceTier},
		{"prompt_cache_key", "promptCacheKey", r.PromptCacheKey},
		{"prompt_cache_retention", "promptCacheRetention", r.PromptCacheRetention},
	}
	switch target.Type {
	case execution.TargetOpenAICompatible:
		for _, field := range fields {
			if field.value != nil {
				values[field.chat] = *field.value
			}
		}
		if r.Metadata != nil {
			values["metadata"] = r.Metadata
		}
		if r.LogitBias != nil {
			values["logit_bias"] = r.LogitBias
		}
		if r.Logprobs != nil {
			values["logprobs"] = *r.Logprobs
		}
		if r.TopLogprobs != nil {
			values["top_logprobs"] = *r.TopLogprobs
		}
		if boolValue(r.Logprobs) {
			values["preserveChatCompletionMetadata"] = true
		}
	case execution.TargetOpenAI:
		if len(r.LogitBias) > 0 || (r.ServiceTier != nil && *r.ServiceTier == "scale") {
			return provider.ErrUnsupportedFunctionality
		}
		for _, field := range fields {
			if field.value != nil {
				values[field.native] = *field.value
			}
		}
		if r.Metadata != nil {
			values["metadata"] = r.Metadata
		}
		if boolValue(r.Logprobs) {
			// Responses' pinned provider normalizer requires a positive alternatives
			// count to retain logprobs. The Chat projector trims to the requested count.
			count := 1
			if r.TopLogprobs != nil {
				count = max(1, *r.TopLogprobs)
			}
			values["logprobs"] = count
		}
	case execution.TargetAnthropic:
		for _, field := range fields {
			if field.value != nil {
				return provider.ErrUnsupportedFunctionality
			}
		}
		if len(r.Metadata) > 0 || len(r.LogitBias) > 0 || boolValue(r.Logprobs) {
			return provider.ErrUnsupportedFunctionality
		}
	}
	return nil
}
