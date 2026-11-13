package middleware

import (
	"context"
	"fmt"
	"maps"

	"github.com/grafana/ai-sdk/internal/provideroptions"
	"github.com/grafana/ai-sdk/provider"
)

// DefaultSettingsOptions defines default values for CallOptions fields.
// Only non-nil/non-zero fields are applied as defaults; caller-provided
// values always take precedence. Reasoning is a Go addition to the upstream
// settings: a caller's zero ReasoningProviderDefault receives the default.
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
	Reasoning        provider.ReasoningEffort
	Tools            []provider.Tool
	ToolChoice       *provider.ToolChoice
	Headers          map[string]string
	ProviderOptions  provider.ProviderOptions
}

// DefaultSettings returns a Middleware that applies fallback values for
// CallOptions fields, following upstream defaultSettingsMiddleware.
//
// Pointer and slice defaults fill only nil caller fields, so a non-nil empty
// caller slice is kept. Headers merge per key. A caller ToolChoice replaces
// the default whole.
//
// ResponseFormat merges per member: the caller's type wins, or the default's
// when the caller's is empty. A JSON result takes an unset schema, name or
// description from the default; any other result takes none of them.
//
// ProviderOptions deep-merge: namespaces that are JSON objects on both sides
// merge recursively and reach the model as a [provider.RawProviderOption],
// which providers read with [provider.ResolveOption]. Arrays, nulls and other
// values replace, the keys __proto__, constructor and prototype are ignored
// in caller objects, and a namespace set on only one side keeps its original
// value. TransformParams returns an error when a shared namespace cannot be
// marshaled.
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
			p.ResponseFormat = mergeResponseFormat(settings.ResponseFormat, p.ResponseFormat)
			if p.Seed == nil && settings.Seed != nil {
				p.Seed = settings.Seed
			}
			if p.Reasoning == provider.ReasoningProviderDefault && settings.Reasoning != provider.ReasoningProviderDefault {
				p.Reasoning = settings.Reasoning
			}
			if p.Tools == nil && settings.Tools != nil {
				p.Tools = settings.Tools
			}
			if p.ToolChoice == nil && settings.ToolChoice != nil {
				p.ToolChoice = settings.ToolChoice
			}

			p.Headers = mergeHeaders(settings.Headers, p.Headers)
			providerOptions, err := provideroptions.MergeOverlapping(settings.ProviderOptions, p.ProviderOptions)
			if err != nil {
				return provider.CallOptions{}, fmt.Errorf("middleware: merging default provider options: %w", err)
			}
			p.ProviderOptions = providerOptions

			return p, nil
		},
	}
}

func mergeResponseFormat(defaults, caller *provider.ResponseFormat) *provider.ResponseFormat {
	if caller == nil {
		return defaults
	}
	if defaults == nil {
		return caller
	}
	merged := *caller
	if merged.Type == "" {
		merged.Type = defaults.Type
	}
	if merged.Type != provider.ResponseFormatJSON {
		return &merged
	}
	if merged.Schema == nil {
		merged.Schema = defaults.Schema
	}
	if merged.Name == nil {
		merged.Name = defaults.Name
	}
	if merged.Description == "" {
		merged.Description = defaults.Description
	}
	return &merged
}

func mergeHeaders(defaults, caller map[string]string) map[string]string {
	if defaults == nil && caller == nil {
		return nil
	}
	merged := make(map[string]string, len(defaults)+len(caller))
	maps.Copy(merged, defaults)
	maps.Copy(merged, caller)
	return merged
}
