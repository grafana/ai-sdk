package v4

import (
	"encoding/json"
	"slices"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

// applyProviderOptionPolicy removes the caller provider options the resolved
// backend does not read, at every level the mapper forwards. It runs after
// resolution, because only the resolved model knows its backend, and it
// refuses nothing: an option for another backend is one every AI SDK provider
// ignores, so dropping it changes nothing the caller could observe from that
// backend. Refusals that do not depend on the backend happen during mapping.
func applyProviderOptionPolicy(options provider.CallOptions, policy catalog.ProviderOptionPolicy) provider.CallOptions {
	options.ProviderOptions = filterProviderOptions(options.ProviderOptions, policy)
	if len(options.Prompt) == 0 {
		return options
	}
	prompt := make([]provider.Message, len(options.Prompt))
	for i, message := range options.Prompt {
		message.ProviderOptions = filterProviderOptions(message.ProviderOptions, policy)
		if len(message.Content) > 0 {
			content := make([]provider.ContentPart, len(message.Content))
			for j, part := range message.Content {
				part.ProviderOptions = filterProviderOptions(part.ProviderOptions, policy)
				content[j] = part
			}
			message.Content = content
		}
		prompt[i] = message
	}
	options.Prompt = prompt
	return options
}

func filterProviderOptions(options provider.ProviderOptions, policy catalog.ProviderOptionPolicy) provider.ProviderOptions {
	if len(options) == 0 {
		return nil
	}
	filtered := make(provider.ProviderOptions, len(options))
	for namespace, value := range options {
		if !slices.Contains(policy.Namespaces, namespace) {
			continue
		}
		if allowed, restricted := policy.Fields[namespace]; restricted {
			restrictedValue, ok := restrictProviderOptionFields(value, allowed)
			if !ok {
				continue
			}
			value = restrictedValue
		}
		filtered[namespace] = value
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// restrictProviderOptionFields keeps only the allowed top-level fields. A
// namespace with nothing to remove is returned unchanged, so its bytes survive
// exactly; otherwise the kept fields are re-encoded with their values intact.
func restrictProviderOptionFields(value provider.ProviderOption, allowed []string) (provider.ProviderOption, bool) {
	raw, ok := value.(provider.RawProviderOption)
	if !ok {
		return nil, false
	}
	fields, ok := jsonObject(raw.Raw)
	if !ok {
		return nil, false
	}
	removed := false
	for field := range fields {
		if !slices.ContainsFunc(allowed, func(name string) bool {
			return normalizeProviderOptionField(name) == normalizeProviderOptionField(field)
		}) {
			delete(fields, field)
			removed = true
		}
	}
	if !removed {
		return raw, true
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, false
	}
	return provider.RawProviderOption{Key: raw.Key, Raw: encoded}, true
}
