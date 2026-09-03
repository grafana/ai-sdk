package catalog

import (
	"context"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type staticCatalog struct {
	namespace modelNamespace
	models    map[string]provider.LanguageModel
}

// NewStatic creates an immutable catalog from fully constructed models.
// It copies entry metadata and rejects invalid IDs, alias collisions, and nil
// models before returning a catalog.
func NewStatic(entries []StaticEntry) (Catalog, error) {
	infos := make([]ModelInfo, len(entries))
	models := make(map[string]provider.LanguageModel, len(entries))

	for i, entry := range entries {
		if isNilInterface(entry.Model) {
			return nil, fmt.Errorf("catalog: model %q is nil", entry.Info.ID)
		}
		infos[i] = entry.Info
		models[entry.Info.ID] = entry.Model
	}

	namespace, err := newModelNamespace(infos)
	if err != nil {
		return nil, err
	}

	return &staticCatalog{
		namespace: namespace,
		models:    models,
	}, nil
}

func (c *staticCatalog) ResolveModel(_ context.Context, modelID string) (ResolvedModel, error) {
	canonicalID, exists := c.namespace.canonicalID(modelID)
	if !exists {
		return ResolvedModel{}, &UnknownModelError{ModelID: modelID}
	}
	return ResolvedModel{ID: canonicalID, Model: c.models[canonicalID]}, nil
}

func (c *staticCatalog) ListModels(_ context.Context) ([]ModelInfo, error) {
	return c.namespace.list(), nil
}
