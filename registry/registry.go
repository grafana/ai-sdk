package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

type ProviderRegistry struct {
	providers   map[string]Provider
	separator   string
	middlewares []middleware.Middleware
}

type RegistryOption func(*ProviderRegistry)

func WithSeparator(sep string) RegistryOption {
	return func(r *ProviderRegistry) { r.separator = sep }
}

func WithLanguageModelMiddleware(mws ...middleware.Middleware) RegistryOption {
	return func(r *ProviderRegistry) { r.middlewares = mws }
}

func NewProviderRegistry(providers map[string]Provider, opts ...RegistryOption) *ProviderRegistry {
	providerMap := make(map[string]Provider, len(providers))
	for id, p := range providers {
		providerMap[id] = p
	}
	r := &ProviderRegistry{
		providers: providerMap,
		separator: ":",
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *ProviderRegistry) LanguageModel(id string) (provider.LanguageModel, error) {
	providerID, modelID, err := r.splitID(id)
	if err != nil {
		return nil, err
	}

	p, err := r.getProvider(providerID)
	if err != nil {
		return nil, err
	}

	model, err := p.LanguageModel(modelID)
	if err != nil {
		return nil, err
	}
	if model == nil {
		return nil, fmt.Errorf("no such language model: %s: %w", id, ErrNoSuchModel)
	}

	if len(r.middlewares) > 0 {
		model = middleware.WrapLanguageModel(model, r.middlewares...)
	}

	return model, nil
}

func (r *ProviderRegistry) splitID(id string) (providerID, modelID string, err error) {
	idx := strings.Index(id, r.separator)
	if idx == -1 {
		return "", "", fmt.Errorf(
			"invalid model ID for registry: %q (must be in the format \"providerId%smodelId\"): %w",
			id, r.separator, ErrInvalidModelID,
		)
	}
	return id[:idx], id[idx+len(r.separator):], nil
}

func (r *ProviderRegistry) getProvider(id string) (Provider, error) {
	p, ok := r.providers[id]
	if !ok || p == nil {
		available := make([]string, 0, len(r.providers))
		for k := range r.providers {
			available = append(available, k)
		}
		sort.Strings(available)
		return nil, fmt.Errorf(
			"no such provider: %q (available providers: %s): %w",
			id, strings.Join(available, ","), ErrNoSuchProvider,
		)
	}
	return p, nil
}
