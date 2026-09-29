package catalog

import (
	"context"
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

// ErrUnsupportedRequest identifies a request rejected by a route's execution policy.
var ErrUnsupportedRequest = errors.New("gateway: unsupported request")

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
	// ProviderOptions limits which caller provider options reach Model.
	ProviderOptions ProviderOptionPolicy
}

// ProviderOptionPolicy limits which caller provider options reach a resolved
// model, at call, message and content-part level. The zero value forwards
// none, so a backend whose options nobody has classified receives none.
type ProviderOptionPolicy struct {
	// Namespaces lists the provider-option namespaces the backend reads,
	// compared exactly. Options in any other namespace are not forwarded.
	Namespaces []string
	// Fields maps a namespace to the only top-level fields forwarded in it,
	// compared after folding case and removing "_" and "-", because providers
	// decode option names that way. A namespace absent from Fields forwards
	// every field.
	Fields map[string][]string
}

// clone returns a deep copy, so a catalog stays immutable after construction.
func (p ProviderOptionPolicy) clone() ProviderOptionPolicy {
	cloned := ProviderOptionPolicy{Namespaces: append([]string(nil), p.Namespaces...)}
	if p.Fields != nil {
		cloned.Fields = make(map[string][]string, len(p.Fields))
		for namespace, fields := range p.Fields {
			cloned.Fields[namespace] = append([]string(nil), fields...)
		}
	}
	return cloned
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
	// ProviderOptions limits which caller provider options reach Model.
	ProviderOptions ProviderOptionPolicy
}

// RegistryRoute maps one public model entry to an opaque provider model ID.
type RegistryRoute struct {
	// Info describes the route's public catalog identity.
	Info ModelInfo
	// ProviderModelID is passed unchanged to registry.Provider.LanguageModel.
	ProviderModelID string
	// ProviderOptions limits which caller provider options reach the model.
	ProviderOptions ProviderOptionPolicy
}
