package catalog

import (
	"context"

	"github.com/grafana/ai-sdk/provider"
)

// ModelResolver resolves public gateway model IDs.
type ModelResolver interface {
	ResolveModel(ctx context.Context, modelID string) (ResolvedModel, error)
}

// ModelLister lists public gateway model metadata visible to a request.
type ModelLister interface {
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// Catalog resolves and lists public gateway models.
type Catalog interface {
	ModelResolver
	ModelLister
}

// ResolvedModel contains a model and its canonical public catalog ID.
type ResolvedModel struct {
	// ID is the canonical public catalog ID, which may differ from Model.ModelID().
	ID string
	// Model is the resolved provider language model.
	Model provider.LanguageModel
}

// ModelCapability identifies behavior guaranteed by a public model route.
type ModelCapability string

// ModelInfo describes a canonical public model route.
type ModelInfo struct {
	// ID is the required canonical public model ID.
	ID string
	// Name is an optional presentation name.
	Name string
	// Description is an optional presentation description.
	Description string
	// Aliases are exact public IDs that resolve to ID.
	Aliases []string
	// Capabilities are behaviors guaranteed by this public route.
	Capabilities []ModelCapability
}

// StaticEntry configures one model in a static catalog.
type StaticEntry struct {
	// Info describes the model's public catalog identity.
	Info ModelInfo
	// Model is the fully constructed model returned during resolution.
	Model provider.LanguageModel
}

// RegistryRoute maps one public model entry to an opaque provider model ID.
type RegistryRoute struct {
	// Info describes the route's public catalog identity.
	Info ModelInfo
	// ProviderModelID is passed unchanged to registry.Provider.LanguageModel.
	ProviderModelID string
}
