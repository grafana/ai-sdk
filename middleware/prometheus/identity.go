package prometheus

import "github.com/grafana/ai-sdk/provider"

type identity struct {
	provider string
	model    string
}

func (i *instrumentation) requestedIdentity(model provider.LanguageModel) identity {
	if model == nil {
		return i.normalizeIdentity(identity{})
	}
	return i.normalizeIdentity(identity{provider: model.Provider(), model: model.ModelID()})
}

func (i *instrumentation) generateFinalIdentity(requested identity, result *provider.GenerateResult) identity {
	if i.config.identitySource == IdentityRequested || result == nil || result.Response == nil {
		return requested
	}
	if result.Response.Provider == "" || result.Response.ModelID == "" {
		return requested
	}
	return i.normalizeIdentity(identity{provider: result.Response.Provider, model: result.Response.ModelID})
}

func (i *instrumentation) streamFinalIdentity(requested identity, response identity) identity {
	if i.config.identitySource == IdentityRequested || response.provider == "" || response.model == "" {
		return requested
	}
	return i.normalizeIdentity(response)
}

func (i *instrumentation) normalizeIdentity(id identity) identity {
	providerLabel := id.provider
	if i.config.normalizeProvider != nil {
		providerLabel = i.config.normalizeProvider(providerLabel)
	}
	modelLabel := id.model
	if i.config.normalizeModel != nil {
		modelLabel = i.config.normalizeModel(providerLabel, modelLabel)
	}
	return identity{provider: providerLabel, model: modelLabel}
}
