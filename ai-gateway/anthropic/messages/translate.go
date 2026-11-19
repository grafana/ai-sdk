package messages

import (
	"encoding/json"
	"maps"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

func messagesTranslator(in request) execution.Translator {
	return func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
		var effort *provider.ReasoningEffort
		if target.Type == execution.TargetAnthropic && in.OutputConfig != nil {
			effort = in.OutputConfig.Effort
		}
		var disabled *bool
		if in.ToolChoice != nil {
			disabled = in.ToolChoice.DisableParallelToolUse
		}
		if effort == nil && disabled == nil {
			return options, nil
		}

		namespace, parallelKey := "anthropic", "disableParallelToolUse"
		invertParallel := false
		switch target.Type {
		case execution.TargetAnthropic:
		case execution.TargetOpenAI:
			namespace, parallelKey = "openai", "parallelToolCalls"
			invertParallel = true
		case execution.TargetOpenAICompatible:
			namespace, _, _ = strings.Cut(target.ProviderName, ".")
			namespace = strings.TrimSpace(namespace)
			if namespace == "" {
				namespace = "openai-compatible"
			}
			parallelKey = "parallel_tool_calls"
			invertParallel = true
		default:
			return options, execution.ErrTranslation
		}

		values := map[string]json.RawMessage{}
		if existing := options.ProviderOptions[namespace]; existing != nil {
			var raw []byte
			switch value := existing.(type) {
			case provider.RawProviderOption:
				raw = value.Raw
			default:
				var err error
				raw, err = json.Marshal(value)
				if err != nil {
					return options, err
				}
			}
			if len(raw) != 0 {
				if err := json.Unmarshal(raw, &values); err != nil {
					return options, err
				}
			}
		}
		if values == nil {
			values = map[string]json.RawMessage{}
		}
		if effort != nil {
			encoded, err := json.Marshal(*effort)
			if err != nil {
				return options, err
			}
			values["effort"] = encoded
		}
		if disabled != nil {
			encoded, err := json.Marshal(*disabled != invertParallel)
			if err != nil {
				return options, err
			}
			values[parallelKey] = encoded
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
