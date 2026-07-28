package registry

import "github.com/grafana/ai-sdk/provider"

type Provider interface {
	LanguageModel(modelID string) (provider.LanguageModel, error)
}
