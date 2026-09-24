package service

import (
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
)

// NativePolicies freezes conservative backend capabilities before wrappers hide
// backend identity. Uncertified model IDs stay available only through ProviderWire.
func NativePolicies(file config.File) map[string]chatcompletions.Backend {
	result := map[string]chatcompletions.Backend{}
	for id, model := range file.Models {
		descriptors := append([]config.Primary{model.Primary}, model.Fallback...)
		policy := chatcompletions.Backend("")
		for i, d := range descriptors {
			configured := file.Providers[d.Provider]
			candidate := nativeBackend(configured.Type, d.Model)
			// Storage overrides must reach the compatible provider's namespace.
			// Custom namespaces need their own certified mapping.
			if configured.Type == "openai-compatible" && configured.ProviderName != "" && configured.ProviderName != "openai-compatible" {
				candidate = ""
			}
			if candidate == "" {
				policy = ""
				break
			}
			if len(descriptors) > 1 {
				if candidate != chatcompletions.BackendAnthropic {
					policy = ""
					break
				}
				policy = chatcompletions.BackendFallback
			} else if i == 0 {
				policy = candidate
			}
		}
		if policy != "" {
			result[id] = policy
		}
	}
	return result
}
func nativeBackend(providerType, model string) chatcompletions.Backend {
	switch providerType {
	case "anthropic":
		switch model {
		case "claude-sonnet-4-20250514", "claude-3-5-haiku-20241022":
			return chatcompletions.BackendAnthropic
		}
	case "openai":
		switch model {
		case "gpt-4.1", "gpt-4.1-mini", "gpt-4o", "gpt-4o-mini":
			return chatcompletions.BackendResponses
		case "o3", "o3-mini", "o4-mini":
			return chatcompletions.BackendReasoning
		}
	case "openai-compatible":
		switch model {
		case "gpt-4o", "gpt-4o-mini":
			return chatcompletions.BackendCompatible
		}
	}
	return ""
}
