package registry

import (
	"errors"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomProvider_LanguageModel(t *testing.T) {
	t.Run("ExplicitModel_ReturnsModel", func(t *testing.T) {
		model := &mockModel{providerName: "test", modelID: "fast"}
		cp := NewCustomProvider(
			WithLanguageModels(map[string]provider.LanguageModel{"fast": model}),
		)

		got, err := cp.LanguageModel("fast")
		require.NoError(t, err)
		assert.Equal(t, model, got)
	})

	t.Run("FallbackProvider_ResolvesUnlistedModel", func(t *testing.T) {
		fallbackModel := &mockModel{providerName: "anthropic", modelID: "claude-sonnet-4-6"}
		fallback := &mockProvider{models: map[string]provider.LanguageModel{"claude-sonnet-4-6": fallbackModel}}

		cp := NewCustomProvider(
			WithLanguageModels(map[string]provider.LanguageModel{
				"fast": &mockModel{modelID: "fast"},
			}),
			WithFallbackProvider(fallback),
		)

		got, err := cp.LanguageModel("claude-sonnet-4-6")
		require.NoError(t, err)
		assert.Equal(t, fallbackModel, got)
	})

	t.Run("ExplicitModel_TakesPriorityOverFallback", func(t *testing.T) {
		explicitModel := &mockModel{modelID: "fast-explicit"}
		fallbackModel := &mockModel{modelID: "fast-fallback"}

		cp := NewCustomProvider(
			WithLanguageModels(map[string]provider.LanguageModel{"fast": explicitModel}),
			WithFallbackProvider(&mockProvider{models: map[string]provider.LanguageModel{"fast": fallbackModel}}),
		)

		got, err := cp.LanguageModel("fast")
		require.NoError(t, err)
		assert.Equal(t, explicitModel, got)
	})

	t.Run("NoFallback_UnknownModel_ReturnsErrNoSuchModel", func(t *testing.T) {
		cp := NewCustomProvider(
			WithLanguageModels(map[string]provider.LanguageModel{
				"fast": &mockModel{modelID: "fast"},
			}),
		)

		_, err := cp.LanguageModel("unknown")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoSuchModel))
	})

	t.Run("NilModelInMap_ReturnsErrNoSuchModel", func(t *testing.T) {
		fallbackModel := &mockModel{modelID: "from-fallback"}
		cp := NewCustomProvider(
			WithLanguageModels(map[string]provider.LanguageModel{"broken": nil}),
			WithFallbackProvider(&mockProvider{models: map[string]provider.LanguageModel{"broken": fallbackModel}}),
		)

		_, err := cp.LanguageModel("broken")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoSuchModel))
	})

	t.Run("LanguageModelMapCopied", func(t *testing.T) {
		model := &mockModel{providerName: "test", modelID: "fast"}
		models := map[string]provider.LanguageModel{"fast": model}
		cp := NewCustomProvider(WithLanguageModels(models))
		delete(models, "fast")

		got, err := cp.LanguageModel("fast")
		require.NoError(t, err)
		assert.Equal(t, model, got)
	})

	t.Run("NilModelInMap_NoFallback_ReturnsErrNoSuchModel", func(t *testing.T) {
		cp := NewCustomProvider(
			WithLanguageModels(map[string]provider.LanguageModel{"broken": nil}),
		)

		_, err := cp.LanguageModel("broken")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoSuchModel))
	})

	t.Run("NoModelsNoFallback_ReturnsErrNoSuchModel", func(t *testing.T) {
		cp := NewCustomProvider()

		_, err := cp.LanguageModel("anything")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoSuchModel))
	})

	t.Run("FallbackFails_PropagatesError", func(t *testing.T) {
		fallback := &mockProvider{models: map[string]provider.LanguageModel{}}

		cp := NewCustomProvider(WithFallbackProvider(fallback))

		_, err := cp.LanguageModel("unknown")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoSuchModel))
	})

	t.Run("AccessControl_OnlyExplicitModelsAllowed", func(t *testing.T) {
		cp := NewCustomProvider(
			WithLanguageModels(map[string]provider.LanguageModel{
				"claude-sonnet-4-6": &mockModel{modelID: "claude-sonnet-4-6"},
			}),
		)

		_, err := cp.LanguageModel("claude-sonnet-4-6")
		require.NoError(t, err)

		_, err = cp.LanguageModel("claude-opus-4-6")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrNoSuchModel))
	})

	t.Run("IntegrationWithRegistry", func(t *testing.T) {
		model := &mockModel{providerName: "anthropic", modelID: "fast"}
		cp := NewCustomProvider(
			WithLanguageModels(map[string]provider.LanguageModel{"fast": model}),
		)
		reg := NewProviderRegistry(map[string]Provider{"my": cp})

		got, err := reg.LanguageModel("my:fast")
		require.NoError(t, err)
		assert.Equal(t, model, got)
	})
}
