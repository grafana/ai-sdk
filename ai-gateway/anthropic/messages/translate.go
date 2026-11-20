package messages

import (
	"encoding/json"
	"maps"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

// messagesTranslator applies the Gateway storage policy and parallel tool
// choice in each target's option namespace.
func messagesTranslator(disableParallel *bool) execution.Translator {
	return func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
		namespace := target.ProviderName
		values := map[string]any{}
		switch target.Type {
		case execution.TargetAnthropic:
			namespace = "anthropic"
			if disableParallel != nil {
				values["disableParallelToolUse"] = *disableParallel
			}
		case execution.TargetOpenAI:
			namespace = "openai"
			values["store"] = false
			if disableParallel != nil {
				values["parallelToolCalls"] = !*disableParallel
			}
		case execution.TargetOpenAICompatible:
			namespace, _, _ = strings.Cut(namespace, ".")
			namespace = strings.TrimSpace(namespace)
			if namespace == "" {
				namespace = "openai-compatible"
			}
			values["store"] = false
			if disableParallel != nil {
				values["parallel_tool_calls"] = !*disableParallel
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

// disableParallelToolUse returns the flag from the request's tool choice.
func (r request) disableParallelToolUse() *bool {
	if r.ToolChoice == nil {
		return nil
	}
	return r.ToolChoice.DisableParallelToolUse
}
