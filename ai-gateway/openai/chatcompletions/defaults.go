package chatcompletions

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

func chatTranslator(r mappedRequest) execution.Translator {
	return func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
		options.RejectUnsupported = true
		if err := r.validateMessageCapabilities(target); err != nil {
			return provider.CallOptions{}, err
		}
		var err error
		options, err = translateMedia(target, options)
		if err != nil {
			return provider.CallOptions{}, err

		}
		if r.needsNativeAudio() && target.Type != execution.TargetOpenAICompatible {
			return provider.CallOptions{}, provider.ErrUnsupportedFunctionality
		}
		if r.hasCustomTools() && target.Type == execution.TargetAnthropic {
			return provider.CallOptions{}, provider.ErrUnsupportedFunctionality
		}
		if options.ChoiceCount > 1 && target.Type != execution.TargetOpenAICompatible {
			return options, catalog.ErrUnsupportedRequest
		}
		options = translateMessages(r, target, options)
		namespace := target.ProviderName
		values := map[string]any{}
		switch target.Type {
		case execution.TargetOpenAI:
			values["preserveRefusal"] = true
			values["preserveResponseMetadata"] = true
			namespace = "openai"
			values["store"] = false
			values["strictJsonSchema"] = r.strictOutput
			if r.Parallel != nil {
				values["parallelToolCalls"] = *r.Parallel
			}
		case execution.TargetAnthropic:
			// Omitting false preserves Anthropic's non-strict default, including
			// models that do not expose a strict tool flag at all.
			options.Tools = slices.Clone(options.Tools)
			for i := range options.Tools {
				if options.Tools[i].Strict != nil && !*options.Tools[i].Strict {
					options.Tools[i].Strict = nil
				}
			}
			namespace = "anthropic"
			if r.Parallel != nil {
				values["disableParallelToolUse"] = !*r.Parallel
			}
		case execution.TargetOpenAICompatible:
			if r.MaxCompletionTokens != nil {
				values["maxTokensField"] = compatible.MaxTokensFieldCompletion
			} else if r.MaxTokens != nil {
				values["maxTokensField"] = compatible.MaxTokensFieldLegacy
			}
			values["preserveChatCompletionMetadata"] = true
			if r.Modalities != nil {
				values["modalities"] = r.Modalities
			}
			if r.Audio != nil {
				values["audio"] = r.Audio
			}

			namespace, _, _ = strings.Cut(namespace, ".")
			namespace = strings.TrimSpace(namespace)
			if namespace == "" {
				namespace = "openai-compatible"
			}
			values["store"] = false
			values["strictJsonSchema"] = r.strictOutput
			if r.Parallel != nil {
				values["parallel_tool_calls"] = *r.Parallel
			}
		default:
			return provider.CallOptions{}, execution.ErrTranslation
		}
		if err := r.translateAllowedTools(target, &options, values); err != nil {
			return provider.CallOptions{}, err
		}
		if err := r.translateParameters(target, values); err != nil {
			return provider.CallOptions{}, err
		}
		if len(values) == 0 {
			return options, nil
		}
		merged := map[string]json.RawMessage{}
		if existing := options.ProviderOptions[namespace]; existing != nil {
			encoded, err := json.Marshal(existing)
			if raw, ok := existing.(provider.RawProviderOption); ok {
				encoded, err = raw.Raw, nil
			}
			if err != nil || json.Unmarshal(encoded, &merged) != nil || merged == nil {
				return options, execution.ErrTranslation
			}
		}
		for key, value := range values {
			encoded, err := json.Marshal(value)
			if err != nil {
				return options, err
			}
			merged[key] = encoded
		}
		raw, err := json.Marshal(merged)
		if err != nil {
			return options, err
		}
		options.ProviderOptions = maps.Clone(options.ProviderOptions)
		if options.ProviderOptions == nil {
			options.ProviderOptions = provider.ProviderOptions{}
		}
		options.ProviderOptions[namespace] = provider.RawProviderOption{Key: namespace, Raw: raw}
		return options, nil
	}
}
