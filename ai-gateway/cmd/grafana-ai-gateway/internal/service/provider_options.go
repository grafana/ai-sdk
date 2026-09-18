package service

import (
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
)

// anthropicOptionPolicy forwards the anthropic namespace, restricted to the
// fields providers/anthropic reads. A field the provider gains later is removed
// until it is classified here, so a new server-side capability starts off.
// mcpServers, container and fallbacks are absent on purpose: the runtime
// refuses them before resolution, because they would run tools or models the
// catalog never granted on the host's credentials.
var anthropicOptionPolicy = catalog.ProviderOptionPolicy{
	Namespaces: []string{"anthropic"},
	Fields: map[string][]string{"anthropic": {
		// AnthropicOptions, read at call level.
		"thinking", "structuredOutputMode", "disableParallelToolUse", "effort", "betas", "taskBudget", "toolStreaming",
		// AnthropicSystemMessageOptions, read on system messages. cacheControl
		// is also read on messages and parts, under either spelling.
		"toolChanges", "cacheControl",
		// Read from message and part options by the request converter.
		"citations", "title", "context", "signature", "redactedData",
	}},
}

// openAICompatibleOptionPolicy forwards the namespaces providers/openai-compatible
// reads for a provider name: the two fixed openai-compatible keys, the name
// before its first dot, and that name in camel case. It mirrors the provider's
// unexported providerOptionsName and toCamelCase, which
// TestOpenAICompatibleOptionPolicy_MatchesTheNamespacesTheProviderReads checks.
//
// Fields are not restricted. Forwarding endpoint-specific fields is this
// provider's purpose; whether the Gateway should restrict them is open on #115.
func openAICompatibleOptionPolicy(providerName string) catalog.ProviderOptionPolicy {
	if providerName == "" {
		providerName = "openai-compatible"
	}
	name, _, _ := strings.Cut(providerName, ".")
	name = strings.TrimSpace(name)
	namespaces := []string{"openai-compatible", "openaiCompatible"}
	for _, candidate := range []string{name, camelCase(name)} {
		if candidate != "" && !slices.Contains(namespaces, candidate) {
			namespaces = append(namespaces, candidate)
		}
	}
	return catalog.ProviderOptionPolicy{Namespaces: namespaces}
}

func camelCase(s string) string {
	var b strings.Builder
	upperNext := false
	for _, r := range s {
		if r == '-' || r == '_' {
			upperNext = true
			continue
		}
		if upperNext && r >= 'a' && r <= 'z' {
			r -= 'a' - 'A'
		}
		b.WriteRune(r)
		upperNext = false
	}
	return b.String()
}
