package catalog

import (
	"context"
	"fmt"

	"github.com/grafana/ai-sdk/registry"
)

type registryCatalog struct {
	namespace modelNamespace
	provider  registry.Provider
	targets   map[string]string
}

// NewRegistry creates an immutable public catalog backed by a registry
// provider. Resolution passes each route's ProviderModelID unchanged to the
// provider and preserves provider errors in the returned error chain.
func NewRegistry(provider registry.Provider, routes []RegistryRoute) (Catalog, error) {
	if isNilInterface(provider) {
		return nil, fmt.Errorf("catalog: registry provider is nil")
	}

	infos := make([]ModelInfo, len(routes))
	targets := make(map[string]string, len(routes))
	for i, route := range routes {
		if route.ProviderModelID == "" {
			return nil, fmt.Errorf("catalog: provider model ID is required for route %q", route.Info.ID)
		}
		infos[i] = route.Info
		targets[route.Info.ID] = route.ProviderModelID
	}

	namespace, err := newModelNamespace(infos)
	if err != nil {
		return nil, err
	}

	return &registryCatalog{
		namespace: namespace,
		provider:  provider,
		targets:   targets,
	}, nil
}

func (c *registryCatalog) ResolveModel(_ context.Context, modelID string) (ResolvedModel, error) {
	canonicalID, exists := c.namespace.canonicalID(modelID)
	if !exists {
		return ResolvedModel{}, &UnknownModelError{ModelID: modelID}
	}

	providerModelID := c.targets[canonicalID]
	model, err := c.provider.LanguageModel(providerModelID)
	if err != nil {
		return ResolvedModel{}, fmt.Errorf("catalog: resolving route %q via provider model %q: %w", canonicalID, providerModelID, err)
	}
	if isNilInterface(model) {
		return ResolvedModel{}, fmt.Errorf("catalog: provider returned nil model for route %q", canonicalID)
	}

	return ResolvedModel{ID: canonicalID, Model: model}, nil
}

func (c *registryCatalog) ListModels(_ context.Context) ([]ModelInfo, error) {
	return c.namespace.list(), nil
}
