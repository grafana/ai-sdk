package catalog

import (
	"context"
	"fmt"
)

type staticCatalog struct {
	namespace modelNamespace
	models    map[string]ResolvedModel
}

// NewStatic creates an immutable catalog from fully constructed models.
// It copies entry metadata and rejects invalid IDs, alias collisions, and nil
// models before returning a catalog.
func NewStatic(entries []StaticEntry) (Catalog, error) {
	infos := make([]ModelInfo, len(entries))
	models := make(map[string]ResolvedModel, len(entries))

	for i, entry := range entries {
		if isNilInterface(entry.Model) {
			return nil, fmt.Errorf("catalog: model %q is nil", entry.Info.ID)
		}
		infos[i] = entry.Info
		models[entry.Info.ID] = ResolvedModel{ID: entry.Info.ID, Model: entry.Model, Candidates: append([]ConfiguredCandidate(nil), entry.Info.Candidates...)}
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
	resolved := c.models[canonicalID]
	resolved.Candidates = append([]ConfiguredCandidate(nil), resolved.Candidates...)
	return resolved, nil
}

func (c *staticCatalog) ListModels(_ context.Context) ([]ModelInfo, error) {
	return c.namespace.list(), nil
}
