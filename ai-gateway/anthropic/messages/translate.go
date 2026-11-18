package messages

import (
	"encoding/json"
	"maps"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

// messagesTranslator applies disable_parallel_tool_use in each target's own
// option namespace, the way the Chat adapter applies parallel_tool_calls. A
// request that does not set the flag passes through unchanged for every target.
func messagesTranslator(disableParallel *bool) execution.Translator {
	return func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
		if disableParallel == nil {
			return options, nil
		}
		namespace := target.ProviderName
		values := map[string]any{}
		switch target.Type {
		case execution.TargetAnthropic:
			namespace = "anthropic"
			values["disableParallelToolUse"] = *disableParallel
		case execution.TargetOpenAI:
			namespace = "openai"
			values["parallelToolCalls"] = !*disableParallel
		case execution.TargetOpenAICompatible:
			namespace, _, _ = strings.Cut(namespace, ".")
			namespace = strings.TrimSpace(namespace)
			if namespace == "" {
				namespace = "openai-compatible"
			}
			values["parallel_tool_calls"] = !*disableParallel
		default:
			return provider.CallOptions{}, execution.ErrTranslation
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
