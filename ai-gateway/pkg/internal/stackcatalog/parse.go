package stackcatalog

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
)

const (
	providerAnthropic  = "anthropic"
	providerOpenAI     = "openai"
	providerCompatible = "openai-compatible"
)

// Parse validates a stack catalog spec whose providers have the secure keys
// in keys, and returns it as a config.File without provider keys. Custom
// endpoints must be https and pass policy's checks that need no DNS lookup;
// the egress client checks the addresses they resolve to when connecting.
func Parse(spec v1alpha1.Spec, keys []string, policy outbound.EgressPolicy) (config.File, error) {
	if err := checkPolicy(spec); err != nil {
		return config.File{}, err
	}
	file := convert(spec)
	if err := file.ValidateShape(); err != nil {
		return config.File{}, err
	}
	if err := checkEndpoints(file, policy); err != nil {
		return config.File{}, err
	}
	if err := checkKeys(file, keys); err != nil {
		return config.File{}, err
	}
	return file, nil
}

func checkPolicy(spec v1alpha1.Spec) error {
	for _, name := range slices.Sorted(maps.Keys(spec.Providers)) {
		if !validName(name) {
			return fmt.Errorf("modelcatalog: provider names must be DNS labels: 1 to 63 lowercase letters, digits or '-', starting and ending with a letter or digit")
		}
		switch spec.Providers[name].Type {
		case providerAnthropic, providerOpenAI, providerCompatible:
		default:
			return fmt.Errorf("modelcatalog: providers.%s.type must be %s, %s or %s", name, providerAnthropic, providerOpenAI, providerCompatible)
		}
	}
	return nil
}

// checkEndpoints checks and normalizes each custom endpoint. Messages name
// the field path, never the URL or its host.
func checkEndpoints(file config.File, policy outbound.EgressPolicy) error {
	for _, name := range slices.Sorted(maps.Keys(file.Providers)) {
		provider := file.Providers[name]
		if provider.BaseURL == "" {
			continue
		}
		endpoint, err := outbound.CheckEndpoint(provider.BaseURL, policy)
		switch {
		case errors.Is(err, outbound.ErrEndpointForm):
			return fmt.Errorf("modelcatalog: providers.%s.baseURL must be an absolute URL without userinfo, query or fragment", name)
		case errors.Is(err, outbound.ErrEndpointScheme):
			return fmt.Errorf("modelcatalog: providers.%s.baseURL must use https", name)
		case errors.Is(err, execution.ErrEgressRefused):
			return fmt.Errorf("modelcatalog: providers.%s.baseURL is refused by the egress policy", name)
		case err != nil:
			return fmt.Errorf("modelcatalog: providers.%s.baseURL host must be a DNS name or an IP address", name)
		}
		provider.BaseURL = endpoint
		file.Providers[name] = provider
	}
	return nil
}

func convert(spec v1alpha1.Spec) config.File {
	file := config.File{Providers: make(map[string]config.Provider, len(spec.Providers)), Models: make(map[string]config.Model, len(spec.Models))}
	for name, provider := range spec.Providers {
		file.Providers[name] = config.Provider{Type: provider.Type, BaseURL: deref(provider.BaseURL), ProviderName: deref(provider.ProviderName), SupportsStructuredOutputs: deref(provider.SupportsStructuredOutputs)}
	}
	for id, model := range spec.Models {
		converted := config.Model{Name: model.Name, Primary: target(model.Primary), Aliases: slices.Clone(model.Aliases)}
		if model.Description != nil {
			converted.Description = *model.Description
		}
		for _, candidate := range model.Fallback {
			converted.Fallback = append(converted.Fallback, target(candidate))
		}
		file.Models[id] = converted
	}
	return file
}

func deref[T any](pointer *T) T {
	if pointer == nil {
		var zero T
		return zero
	}
	return *pointer
}

func target(value v1alpha1.Target) config.Primary {
	return config.Primary{Provider: value.Provider, Model: value.Model}
}

func checkKeys(file config.File, keys []string) error {
	for _, name := range slices.Sorted(maps.Keys(file.Providers)) {
		if !slices.Contains(keys, name) {
			return fmt.Errorf("modelcatalog: providers.%s requires the secure value secure.%s", name, name)
		}
	}
	for _, key := range slices.Sorted(slices.Values(keys)) {
		if _, ok := file.Providers[key]; !ok {
			return fmt.Errorf("modelcatalog: %s matches no provider", securePath(key))
		}
	}
	return nil
}

func validName(name string) bool { return len(validation.IsDNS1123Label(name)) == 0 }

// securePath names a secure key in errors. Keys that are not provider names
// are not echoed.
func securePath(key string) string {
	if validName(key) {
		return "secure." + key
	}
	return "a secure key that is not a provider name"
}
