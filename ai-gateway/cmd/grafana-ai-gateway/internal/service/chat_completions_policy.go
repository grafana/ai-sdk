package service

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
	"github.com/grafana/ai-sdk/provider"
)

type chatCompletionsBackend string

const (
	chatCompletionsAnthropic  chatCompletionsBackend = "anthropic"
	chatCompletionsResponses  chatCompletionsBackend = "responses"
	chatCompletionsReasoning  chatCompletionsBackend = "responses-reasoning"
	chatCompletionsCompatible chatCompletionsBackend = "compatible"
	chatCompletionsFallback   chatCompletionsBackend = "fallback-text"
)

// ChatCompletionsPolicies freezes conservative route capabilities before
// wrappers hide backend identity. The adapter receives only canonical policy
// functions; uncertified model IDs remain available only through ProviderWire.
func ChatCompletionsPolicies(file config.File) map[string]chatcompletions.RequestPolicy {
	result := map[string]chatcompletions.RequestPolicy{}
	for id, model := range file.Models {
		descriptors := append([]config.Primary{model.Primary}, model.Fallback...)
		backend := chatCompletionsBackend("")
		for i, d := range descriptors {
			configured := file.Providers[d.Provider]
			candidate := chatCompletionsBackendFor(configured.Type, d.Model)
			// Storage overrides must reach the compatible provider's namespace.
			// Custom namespaces need their own certified mapping.
			if configured.Type == "openai-compatible" && configured.ProviderName != "" && configured.ProviderName != "openai-compatible" {
				candidate = ""
			}
			if candidate == "" {
				backend = ""
				break
			}
			if len(descriptors) > 1 {
				if candidate != chatCompletionsAnthropic {
					backend = ""
					break
				}
				backend = chatCompletionsFallback
			} else if i == 0 {
				backend = candidate
			}
		}
		if backend != "" {
			result[id] = chatCompletionsPolicy(backend)
		}
	}
	return result
}

func chatCompletionsBackendFor(providerType, model string) chatCompletionsBackend {
	switch providerType {
	case "anthropic":
		switch model {
		case "claude-sonnet-4-20250514", "claude-3-5-haiku-20241022":
			return chatCompletionsAnthropic
		}
	case "openai":
		switch model {
		case "gpt-4.1", "gpt-4.1-mini", "gpt-4o", "gpt-4o-mini":
			return chatCompletionsResponses
		case "o3", "o3-mini", "o4-mini":
			return chatCompletionsReasoning
		}
	case "openai-compatible":
		switch model {
		case "gpt-4o", "gpt-4o-mini":
			return chatCompletionsCompatible
		}
	}
	return ""
}

func chatCompletionsPolicy(backend chatCompletionsBackend) chatcompletions.RequestPolicy {
	return func(options *provider.CallOptions, requirements chatcompletions.Requirements) error {
		if backend == chatCompletionsFallback && (len(options.Tools) > 0 || requirements.History || requirements.HasParallelTools || requirements.JSONOutput || options.Reasoning != "" || options.ToolChoice != nil && options.ToolChoice.Type != provider.ToolChoiceAuto) {
			return catalogUnsupported()
		}
		if (backend == chatCompletionsAnthropic || backend == chatCompletionsFallback) && (options.FrequencyPenalty != nil || options.PresencePenalty != nil || options.Seed != nil || requirements.HasParallelTools || requirements.JSONOutput || options.Reasoning != "") {
			return catalogUnsupported()
		}
		if backend == chatCompletionsAnthropic || backend == chatCompletionsFallback {
			if options.MaxOutputTokens == nil {
				value := 4096
				options.MaxOutputTokens = &value
			} else if *options.MaxOutputTokens > 4096 {
				return catalogUnsupported()
			}
			if options.Temperature != nil && *options.Temperature > 1 {
				return catalogUnsupported()
			}
			for i, tool := range options.Tools {
				if tool.Strict != nil && *tool.Strict {
					return catalogUnsupported()
				}
				options.Tools[i].Strict = nil
			}
		}
		if backend == chatCompletionsCompatible && (requirements.HasParallelTools || options.Reasoning != "" || requirements.JSONOutput) {
			return catalogUnsupported()
		}
		if backend == chatCompletionsCompatible {
			options.ProviderOptions = provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"store":false}`)}}
		}
		if backend == chatCompletionsReasoning && (len(options.Tools) > 0 || requirements.History || options.Temperature != nil || options.TopP != nil) {
			return catalogUnsupported()
		}
		if backend == chatCompletionsResponses || backend == chatCompletionsReasoning {
			if options.Seed != nil || options.PresencePenalty != nil || options.FrequencyPenalty != nil || len(options.StopSequences) > 0 {
				return catalogUnsupported()
			}
			if options.Reasoning != "" {
				if backend != chatCompletionsReasoning || options.Reasoning != provider.ReasoningLow && options.Reasoning != provider.ReasoningMedium && options.Reasoning != provider.ReasoningHigh {
					return catalogUnsupported()
				}
			}
			values := map[string]any{"store": false, "strictJsonSchema": requirements.StrictJSONOutput}
			if requirements.HasParallelTools {
				values["parallelToolCalls"] = requirements.ParallelToolCalls
			}
			encoded, _ := json.Marshal(values)
			options.ProviderOptions = provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: encoded}}
		}
		return nil
	}
}

func catalogUnsupported() error { return catalog.ErrUnsupportedRequest }
