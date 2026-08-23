package middleware

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSettings(t *testing.T) {
	t.Run("DefaultApplied_WhenCallerOmitsField", func(t *testing.T) {
		var received provider.CallOptions
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				received = params
				return &provider.GenerateResult{}, nil
			},
		}

		mw := DefaultSettings(DefaultSettingsOptions{
			Temperature:     ptr(0.7),
			MaxOutputTokens: ptr(1024),
		})

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		require.NotNil(t, received.Temperature)
		assert.Equal(t, 0.7, *received.Temperature)
		require.NotNil(t, received.MaxOutputTokens)
		assert.Equal(t, 1024, *received.MaxOutputTokens)
	})

	t.Run("CallerValueTakesPrecedence", func(t *testing.T) {
		var received provider.CallOptions
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				received = params
				return &provider.GenerateResult{}, nil
			},
		}

		mw := DefaultSettings(DefaultSettingsOptions{
			Temperature: ptr(0.7),
		})

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
			Temperature: ptr(0.3),
		})
		require.NoError(t, err)
		require.NotNil(t, received.Temperature)
		assert.Equal(t, 0.3, *received.Temperature)
	})

	t.Run("MultipleDefaults_PartialOverride", func(t *testing.T) {
		var received provider.CallOptions
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				received = params
				return &provider.GenerateResult{}, nil
			},
		}

		mw := DefaultSettings(DefaultSettingsOptions{
			Temperature:     ptr(0.7),
			MaxOutputTokens: ptr(1024),
			StopSequences:   []string{"STOP"},
		})

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
			Temperature: ptr(0.3),
		})
		require.NoError(t, err)

		assert.Equal(t, 0.3, *received.Temperature)
		assert.Equal(t, 1024, *received.MaxOutputTokens)
		assert.Equal(t, []string{"STOP"}, received.StopSequences)
	})

	t.Run("HeadersMerge", func(t *testing.T) {
		var received provider.CallOptions
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				received = params
				return &provider.GenerateResult{}, nil
			},
		}

		mw := DefaultSettings(DefaultSettingsOptions{
			Headers: map[string]string{
				"x-default": "yes",
				"x-shared":  "from-default",
			},
		})

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
			Headers: map[string]string{
				"x-caller": "yes",
				"x-shared": "from-caller",
			},
		})
		require.NoError(t, err)

		assert.Equal(t, "yes", received.Headers["x-default"])
		assert.Equal(t, "yes", received.Headers["x-caller"])
		assert.Equal(t, "from-caller", received.Headers["x-shared"])
	})

	t.Run("ProviderOptionsMerge", func(t *testing.T) {
		var received provider.CallOptions
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				received = params
				return &provider.GenerateResult{}, nil
			},
		}

		mw := DefaultSettings(DefaultSettingsOptions{
			ProviderOptions: map[string]provider.ProviderOption{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":true}`)},
			},
		})

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
			ProviderOptions: map[string]provider.ProviderOption{
				"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"effort":"high"}`)},
				"openai":    provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"logprobs":true}`)},
			},
		})
		require.NoError(t, err)

		anthRaw := received.ProviderOptions["anthropic"].(provider.RawProviderOption)
		oaiRaw := received.ProviderOptions["openai"].(provider.RawProviderOption)
		assert.JSONEq(t, `{"effort":"high"}`, string(anthRaw.Raw))
		assert.JSONEq(t, `{"logprobs":true}`, string(oaiRaw.Raw))
	})

	t.Run("NilMaps_NoDefaultsOrCaller", func(t *testing.T) {
		var received provider.CallOptions
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				received = params
				return &provider.GenerateResult{}, nil
			},
		}

		mw := DefaultSettings(DefaultSettingsOptions{})
		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		assert.Nil(t, received.Headers)
		assert.Nil(t, received.ProviderOptions)
	})

	t.Run("ExplicitZeroTemperature_TakesPrecedence", func(t *testing.T) {
		var received provider.CallOptions
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				received = params
				return &provider.GenerateResult{}, nil
			},
		}

		mw := DefaultSettings(DefaultSettingsOptions{
			Temperature: ptr(0.7),
		})

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
			Temperature: ptr(0.0),
		})
		require.NoError(t, err)
		require.NotNil(t, received.Temperature)
		assert.Equal(t, 0.0, *received.Temperature)
	})

	t.Run("AllPointerFields", func(t *testing.T) {
		var received provider.CallOptions
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				received = params
				return &provider.GenerateResult{}, nil
			},
		}

		mw := DefaultSettings(DefaultSettingsOptions{
			TopP:             ptr(0.9),
			TopK:             ptr(40),
			PresencePenalty:  ptr(0.1),
			FrequencyPenalty: ptr(0.2),
			Seed:             ptr(42),
			Reasoning:        ptr(provider.ReasoningMedium),
		})

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		assert.Equal(t, 0.9, *received.TopP)
		assert.Equal(t, 40, *received.TopK)
		assert.Equal(t, 0.1, *received.PresencePenalty)
		assert.Equal(t, 0.2, *received.FrequencyPenalty)
		assert.Equal(t, 42, *received.Seed)
		assert.Equal(t, provider.ReasoningMedium, received.Reasoning)
	})

	t.Run("CallerReasoningTakesPrecedence", func(t *testing.T) {
		var received provider.CallOptions
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				received = params
				return &provider.GenerateResult{}, nil
			},
		}

		mw := DefaultSettings(DefaultSettingsOptions{Reasoning: ptr(provider.ReasoningMedium)})
		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{Reasoning: provider.ReasoningHigh})
		require.NoError(t, err)
		assert.Equal(t, provider.ReasoningHigh, received.Reasoning)
	})
}
