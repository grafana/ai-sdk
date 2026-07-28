package middleware

import (
	"context"

	"github.com/grafana/ai-sdk/provider"
)

// DefaultSettingsOptions defines default values for CallOptions fields.
// Only non-nil/non-zero fields are applied as defaults; caller-provided
// values always take precedence.
type DefaultSettingsOptions struct {
	MaxOutputTokens  *int
	Temperature      *float64
	TopP             *float64
	TopK             *int
	PresencePenalty  *float64
	FrequencyPenalty *float64
	StopSequences    []string
	ResponseFormat   *provider.ResponseFormat
	Seed             *int
	Reasoning        *provider.ReasoningEffort
	Tools            []provider.Tool
	ToolChoice       *provider.ToolChoice
	Headers          map[string]string
	ProviderOptions  provider.ProviderOptions
}

// DefaultSettings returns a Middleware that applies fallback values for
// CallOptions fields. Caller-provided values take precedence over defaults.
func DefaultSettings(settings DefaultSettingsOptions) Middleware {
	return Middleware{
		TransformParams: func(_ context.Context, input TransformParamsInput) (provider.CallOptions, error) {
			p := input.Params

			if p.MaxOutputTokens == nil && settings.MaxOutputTokens != nil {
				p.MaxOutputTokens = settings.MaxOutputTokens
			}
			if p.Temperature == nil && settings.Temperature != nil {
				p.Temperature = settings.Temperature
			}
			if p.TopP == nil && settings.TopP != nil {
				p.TopP = settings.TopP
			}
			if p.TopK == nil && settings.TopK != nil {
				p.TopK = settings.TopK
			}
			if p.PresencePenalty == nil && settings.PresencePenalty != nil {
				p.PresencePenalty = settings.PresencePenalty
			}
			if p.FrequencyPenalty == nil && settings.FrequencyPenalty != nil {
				p.FrequencyPenalty = settings.FrequencyPenalty
			}
			if p.StopSequences == nil && settings.StopSequences != nil {
				p.StopSequences = settings.StopSequences
			}
			if p.ResponseFormat == nil && settings.ResponseFormat != nil {
				p.ResponseFormat = settings.ResponseFormat
			}
			if p.Seed == nil && settings.Seed != nil {
				p.Seed = settings.Seed
			}
			if p.Reasoning == nil && settings.Reasoning != nil {
				p.Reasoning = settings.Reasoning
			}
			if p.Tools == nil && settings.Tools != nil {
				p.Tools = settings.Tools
			}
			if p.ToolChoice == nil && settings.ToolChoice != nil {
				p.ToolChoice = settings.ToolChoice
			}

			p.Headers = mergeMaps(settings.Headers, p.Headers)
			p.ProviderOptions = mergeMaps(settings.ProviderOptions, p.ProviderOptions)

			return p, nil
		},
	}
}

func mergeMaps[M ~map[string]V, V any](defaults, caller M) M {
	if defaults == nil && caller == nil {
		return nil
	}
	merged := make(M, len(defaults)+len(caller))
	for k, v := range defaults {
		merged[k] = v
	}
	for k, v := range caller {
		merged[k] = v
	}
	return merged
}
