package registry

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ Provider = (*ProviderRegistry)(nil)
	_ Provider = (*customProvider)(nil)
)

type mockModel struct {
	providerName string
	modelID      string
}

func (m *mockModel) SpecificationVersion() string               { return "v4" }
func (m *mockModel) Provider() string                           { return m.providerName }
func (m *mockModel) ModelID() string                            { return m.modelID }
func (m *mockModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }

func (m *mockModel) DoGenerate(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
	return &provider.GenerateResult{}, nil
}

func (m *mockModel) DoStream(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	ch := make(chan provider.StreamPart)
	close(ch)
	return &provider.StreamResult{Stream: ch}, nil
}

type mockProvider struct {
	models    map[string]provider.LanguageModel
	callCount int
	lastID    string
}

func (p *mockProvider) LanguageModel(modelID string) (provider.LanguageModel, error) {
	p.callCount++
	p.lastID = modelID
	if m, ok := p.models[modelID]; ok {
		return m, nil
	}
	return nil, ErrNoSuchModel
}

func TestProviderRegistry_LanguageModel(t *testing.T) {
	t.Run("ResolvesModelFromProvider", func(t *testing.T) {
		model := &mockModel{providerName: "anthropic", modelID: "claude-sonnet-4-6"}
		p := &mockProvider{models: map[string]provider.LanguageModel{"claude-sonnet-4-6": model}}
		reg := NewProviderRegistry(map[string]Provider{"anthropic": p})

		got, err := reg.LanguageModel("anthropic:claude-sonnet-4-6")
		require.NoError(t, err)
		assert.Equal(t, model, got)
		assert.Equal(t, "claude-sonnet-4-6", p.lastID)
	})

	t.Run("ModelIDContainsSeparator", func(t *testing.T) {
		model := &mockModel{modelID: "model:part2"}
		p := &mockProvider{models: map[string]provider.LanguageModel{"model:part2": model}}
		reg := NewProviderRegistry(map[string]Provider{"provider": p})

		got, err := reg.LanguageModel("provider:model:part2")
		require.NoError(t, err)
		assert.Equal(t, model, got)
		assert.Equal(t, "model:part2", p.lastID)
	})

	t.Run("MissingSeparator_ReturnsErrInvalidModelID", func(t *testing.T) {
		reg := NewProviderRegistry(map[string]Provider{})

		_, err := reg.LanguageModel("no-separator")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidModelID))
		assert.True(t, errors.Is(err, ErrNoSuchModel), "ErrInvalidModelID should also match ErrNoSuchModel")
		assert.Contains(t, err.Error(), "must be in the format")
	})

	t.Run("UnknownProvider_ReturnsErrNoSuchProvider", func(t *testing.T) {
		reg := NewProviderRegistry(map[string]Provider{
			"anthropic": &mockProvider{},
		})

		_, err := reg.LanguageModel("unknown:model")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoSuchProvider))
		assert.True(t, errors.Is(err, ErrNoSuchModel), "ErrNoSuchProvider should also match ErrNoSuchModel")
		assert.Contains(t, err.Error(), "unknown")
		assert.Contains(t, err.Error(), "anthropic")
	})

	t.Run("NilProviderInMap_ReturnsErrNoSuchProvider", func(t *testing.T) {
		reg := NewProviderRegistry(map[string]Provider{"broken": nil})

		_, err := reg.LanguageModel("broken:model")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoSuchProvider))
	})

	t.Run("ProviderReturnsNilModel_ReturnsErrNoSuchModel", func(t *testing.T) {
		p := &mockProvider{models: map[string]provider.LanguageModel{}}
		reg := NewProviderRegistry(map[string]Provider{"provider": p})

		_, err := reg.LanguageModel("provider:nonexistent")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoSuchModel))
	})

	t.Run("ProviderMapCopied", func(t *testing.T) {
		model := &mockModel{providerName: "provider", modelID: "model"}
		p := &mockProvider{models: map[string]provider.LanguageModel{"model": model}}
		providers := map[string]Provider{"provider": p}
		reg := NewProviderRegistry(providers)
		delete(providers, "provider")

		got, err := reg.LanguageModel("provider:model")
		require.NoError(t, err)
		assert.Equal(t, model, got)
	})

	t.Run("CustomSeparator_SingleChar", func(t *testing.T) {
		model := &mockModel{modelID: "model"}
		p := &mockProvider{models: map[string]provider.LanguageModel{"model": model}}
		reg := NewProviderRegistry(
			map[string]Provider{"provider": p},
			WithSeparator("|"),
		)

		got, err := reg.LanguageModel("provider|model")
		require.NoError(t, err)
		assert.Equal(t, model, got)
	})

	t.Run("CustomSeparator_MultiChar", func(t *testing.T) {
		model := &mockModel{modelID: "model"}
		p := &mockProvider{models: map[string]provider.LanguageModel{"model": model}}
		reg := NewProviderRegistry(
			map[string]Provider{"provider": p},
			WithSeparator(" > "),
		)

		got, err := reg.LanguageModel("provider > model")
		require.NoError(t, err)
		assert.Equal(t, model, got)
	})

	t.Run("MiddlewareWrapsResolvedModel", func(t *testing.T) {
		model := &mockModel{providerName: "test", modelID: "m1"}
		p := &mockProvider{models: map[string]provider.LanguageModel{"m1": model}}
		reg := NewProviderRegistry(
			map[string]Provider{"test": p},
			WithLanguageModelMiddleware(middleware.Middleware{
				OverrideModelID: func(m provider.LanguageModel) string {
					return "overridden-" + m.ModelID()
				},
			}),
		)

		got, err := reg.LanguageModel("test:m1")
		require.NoError(t, err)
		assert.Equal(t, "overridden-m1", got.ModelID())
	})

	t.Run("NoMiddleware_ReturnsUnwrappedModel", func(t *testing.T) {
		model := &mockModel{providerName: "test", modelID: "m1"}
		p := &mockProvider{models: map[string]provider.LanguageModel{"m1": model}}
		reg := NewProviderRegistry(map[string]Provider{"test": p})

		got, err := reg.LanguageModel("test:m1")
		require.NoError(t, err)
		assert.Equal(t, model, got)
	})

	t.Run("MiddlewareAppliedToMultipleProviders", func(t *testing.T) {
		model1 := &mockModel{providerName: "p1", modelID: "m1"}
		model2 := &mockModel{providerName: "p2", modelID: "m2"}
		p1 := &mockProvider{models: map[string]provider.LanguageModel{"m1": model1}}
		p2 := &mockProvider{models: map[string]provider.LanguageModel{"m2": model2}}
		reg := NewProviderRegistry(
			map[string]Provider{"p1": p1, "p2": p2},
			WithLanguageModelMiddleware(middleware.Middleware{
				OverrideModelID: func(m provider.LanguageModel) string {
					return "wrapped-" + m.ModelID()
				},
			}),
		)

		got1, err := reg.LanguageModel("p1:m1")
		require.NoError(t, err)
		assert.Equal(t, "wrapped-m1", got1.ModelID())

		got2, err := reg.LanguageModel("p2:m2")
		require.NoError(t, err)
		assert.Equal(t, "wrapped-m2", got2.ModelID())
	})

	t.Run("Composability_NestedRegistries", func(t *testing.T) {
		model := &mockModel{providerName: "anthropic", modelID: "claude-sonnet-4-6"}
		p := &mockProvider{models: map[string]provider.LanguageModel{"claude-sonnet-4-6": model}}
		inner := NewProviderRegistry(map[string]Provider{"anthropic": p})

		outer := NewProviderRegistry(map[string]Provider{"inner": inner})

		got, err := outer.LanguageModel("inner:anthropic:claude-sonnet-4-6")
		require.NoError(t, err)
		assert.Equal(t, model, got)
	})
}
