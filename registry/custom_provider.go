package registry

import (
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type customProvider struct {
	languageModels   map[string]provider.LanguageModel
	fallbackProvider Provider
}

type CustomProviderOption func(*customProvider)

func WithLanguageModels(models map[string]provider.LanguageModel) CustomProviderOption {
	return func(cp *customProvider) {
		cp.languageModels = make(map[string]provider.LanguageModel, len(models))
		for id, model := range models {
			cp.languageModels[id] = model
		}
	}
}

func WithFallbackProvider(p Provider) CustomProviderOption {
	return func(cp *customProvider) { cp.fallbackProvider = p }
}

func NewCustomProvider(opts ...CustomProviderOption) Provider {
	cp := &customProvider{}
	for _, opt := range opts {
		opt(cp)
	}
	return cp
}

func (cp *customProvider) LanguageModel(modelID string) (provider.LanguageModel, error) {
	if cp.languageModels != nil {
		if model, ok := cp.languageModels[modelID]; ok {
			if model != nil {
				return model, nil
			}
			return nil, fmt.Errorf("no such language model: %q: %w", modelID, ErrNoSuchModel)
		}
	}

	if cp.fallbackProvider != nil {
		return cp.fallbackProvider.LanguageModel(modelID)
	}

	return nil, fmt.Errorf("no such language model: %q: %w", modelID, ErrNoSuchModel)
}
