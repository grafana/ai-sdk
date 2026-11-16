package chatcompletions

import (
	"encoding/json"
	"maps"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

func chatTranslator(r mappedRequest) execution.Translator {
	return func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
		namespace := target.ProviderName
		values := map[string]any{}
		switch target.Type {
		case execution.TargetOpenAI:
			namespace = "openai"
			values["store"] = false
			values["strictJsonSchema"] = r.strictOutput
			if r.Parallel != nil {
				values["parallelToolCalls"] = *r.Parallel
			}
		case execution.TargetAnthropic:
			namespace = "anthropic"
			if r.Parallel != nil {
				values["disableParallelToolUse"] = !*r.Parallel
			}
		case execution.TargetOpenAICompatible:
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
		if len(values) == 0 {
			return options, nil
		}
		raw, err := json.Marshal(values)
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
