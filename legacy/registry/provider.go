package registry

import "github.com/grafana/ai-sdk/legacy/provider"

type Provider interface {
	LanguageModel(modelID string) (provider.LanguageModel, error)
}
