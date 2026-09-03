package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ Catalog           = (*registryCatalog)(nil)
	_ registry.Provider = (*catalogTestProvider)(nil)
)

type catalogTestProvider struct {
	resolve   func(modelID string) (provider.LanguageModel, error)
	callCount int
	modelIDs  []string
}

func (p *catalogTestProvider) LanguageModel(modelID string) (provider.LanguageModel, error) {
	p.callCount++
	p.modelIDs = append(p.modelIDs, modelID)
	return p.resolve(modelID)
}

func TestNewRegistry_Validation(t *testing.T) {
	provider := &catalogTestProvider{resolve: func(string) (provider.LanguageModel, error) {
		return &catalogTestModel{modelID: "native"}, nil
	}}
	var nilProvider *catalogTestProvider

	tests := []struct {
		name        string
		provider    registry.Provider
		routes      []RegistryRoute
		errContains string
	}{
		{
			name:        "NilProvider",
			provider:    nil,
			errContains: "provider is nil",
		},
		{
			name:        "TypedNilProvider",
			provider:    nilProvider,
			errContains: "provider is nil",
		},
		{
			name:        "EmptyProviderModelID",
			provider:    provider,
			routes:      []RegistryRoute{{Info: ModelInfo{ID: "balanced"}}},
			errContains: "balanced",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			catalog, err := NewRegistry(tc.provider, tc.routes)
			require.Error(t, err)
			assert.Nil(t, catalog)
			assert.Contains(t, err.Error(), tc.errContains)
		})
	}
}

func TestRegistryCatalog_ResolveModel(t *testing.T) {
	model := &catalogTestModel{providerName: "anthropic", modelID: "claude-native"}
	provider := &catalogTestProvider{resolve: func(modelID string) (provider.LanguageModel, error) {
		assert.Equal(t, "anthropic:claude-sonnet", modelID)
		return model, nil
	}}
	catalog, err := NewRegistry(provider, []RegistryRoute{{
		Info: ModelInfo{
			ID:      "balanced",
			Aliases: []string{"default"},
		},
		ProviderModelID: "anthropic:claude-sonnet",
	}})
	require.NoError(t, err)

	for _, modelID := range []string{"balanced", "default"} {
		resolved, err := catalog.ResolveModel(context.Background(), modelID)
		require.NoError(t, err)
		assert.Equal(t, "balanced", resolved.ID)
		assert.Same(t, model, resolved.Model)
	}

	assert.Equal(t, []string{
		"anthropic:claude-sonnet",
		"anthropic:claude-sonnet",
	}, provider.modelIDs)
}

func TestRegistryCatalog_CustomProviderOpaqueID(t *testing.T) {
	model := &catalogTestModel{modelID: "native"}
	provider := &catalogTestProvider{resolve: func(modelID string) (provider.LanguageModel, error) {
		assert.Equal(t, "opaque/id without separator", modelID)
		return model, nil
	}}
	catalog, err := NewRegistry(provider, []RegistryRoute{{
		Info:            ModelInfo{ID: "flat-public-id"},
		ProviderModelID: "opaque/id without separator",
	}})
	require.NoError(t, err)

	resolved, err := catalog.ResolveModel(context.Background(), "flat-public-id")
	require.NoError(t, err)
	assert.Equal(t, "flat-public-id", resolved.ID)
	assert.Same(t, model, resolved.Model)
}

func TestRegistryCatalog_NestedProviderRegistry(t *testing.T) {
	model := &catalogTestModel{modelID: "claude-native"}
	leaf := &catalogTestProvider{resolve: func(modelID string) (provider.LanguageModel, error) {
		assert.Equal(t, "claude", modelID)
		return model, nil
	}}
	inner := registry.NewProviderRegistry(map[string]registry.Provider{"anthropic": leaf})
	outer := registry.NewProviderRegistry(map[string]registry.Provider{"inner": inner})
	catalog, err := NewRegistry(outer, []RegistryRoute{{
		Info:            ModelInfo{ID: "balanced"},
		ProviderModelID: "inner:anthropic:claude",
	}})
	require.NoError(t, err)

	resolved, err := catalog.ResolveModel(context.Background(), "balanced")
	require.NoError(t, err)
	assert.Same(t, model, resolved.Model)
	assert.Equal(t, "balanced", resolved.ID)
}

func TestRegistryCatalog_ListModels(t *testing.T) {
	provider := &catalogTestProvider{resolve: func(string) (provider.LanguageModel, error) {
		return &catalogTestModel{modelID: "native"}, nil
	}}
	routes := []RegistryRoute{
		{
			Info:            ModelInfo{ID: "zeta"},
			ProviderModelID: "provider:zeta",
		},
		{
			Info: ModelInfo{
				ID:           "alpha",
				Aliases:      []string{"default"},
				Capabilities: []ModelCapability{"tools"},
			},
			ProviderModelID: "provider:alpha",
		},
	}
	catalog, err := NewRegistry(provider, routes)
	require.NoError(t, err)

	routes[1].Info.ID = "changed"
	routes[1].Info.Aliases[0] = "changed-alias"
	routes[1].Info.Capabilities[0] = "changed-capability"
	routes[1].ProviderModelID = "changed-target"

	models, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []ModelInfo{
		{ID: "alpha", Aliases: []string{"default"}, Capabilities: []ModelCapability{"tools"}},
		{ID: "zeta"},
	}, models)

	models[0].Aliases[0] = "mutated"
	again, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"default"}, again[0].Aliases)

	_, err = catalog.ResolveModel(context.Background(), "default")
	require.NoError(t, err)
	assert.Equal(t, "provider:alpha", provider.modelIDs[0])
}

func TestRegistryCatalog_DownstreamError(t *testing.T) {
	downstreamErr := errors.New("provider unavailable")
	provider := &catalogTestProvider{resolve: func(string) (provider.LanguageModel, error) {
		return nil, downstreamErr
	}}
	catalog, err := NewRegistry(provider, []RegistryRoute{{
		Info:            ModelInfo{ID: "balanced"},
		ProviderModelID: "provider:model",
	}})
	require.NoError(t, err)

	resolved, err := catalog.ResolveModel(context.Background(), "balanced")
	require.Error(t, err)
	assert.Equal(t, ResolvedModel{}, resolved)
	assert.ErrorIs(t, err, downstreamErr)
	assert.NotErrorIs(t, err, ErrUnknownModel)
	assert.Contains(t, err.Error(), "balanced")
}

func TestRegistryCatalog_NilProviderResult(t *testing.T) {
	tests := []struct {
		name  string
		model provider.LanguageModel
	}{
		{name: "NilInterface", model: nil},
		{name: "TypedNil", model: (*catalogTestModel)(nil)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := &catalogTestProvider{resolve: func(string) (provider.LanguageModel, error) {
				return tc.model, nil
			}}
			catalog, err := NewRegistry(provider, []RegistryRoute{{
				Info:            ModelInfo{ID: "balanced"},
				ProviderModelID: "provider:model",
			}})
			require.NoError(t, err)

			resolved, err := catalog.ResolveModel(context.Background(), "balanced")
			require.Error(t, err)
			assert.Equal(t, ResolvedModel{}, resolved)
			assert.NotErrorIs(t, err, ErrUnknownModel)
			assert.Contains(t, err.Error(), "nil model")
		})
	}
}

func TestRegistryCatalog_UnknownPublicRoute(t *testing.T) {
	provider := &catalogTestProvider{resolve: func(string) (provider.LanguageModel, error) {
		return &catalogTestModel{modelID: "native"}, nil
	}}
	catalog, err := NewRegistry(provider, []RegistryRoute{{
		Info:            ModelInfo{ID: "balanced"},
		ProviderModelID: "provider:model",
	}})
	require.NoError(t, err)

	resolved, err := catalog.ResolveModel(context.Background(), "missing")
	require.Error(t, err)
	assert.Equal(t, ResolvedModel{}, resolved)
	assert.ErrorIs(t, err, ErrUnknownModel)
	assert.Zero(t, provider.callCount)
}
