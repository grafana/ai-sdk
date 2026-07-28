package middleware

import (
	"context"
	"errors"
	"testing"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposition_MiddlewareWithFallback(t *testing.T) {
	t.Run("MiddlewareWrappingFallbackModel", func(t *testing.T) {
		primary := &mockModel{
			providerName: "primary",
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, errors.New("primary failed")
			},
		}
		secondary := &mockModel{
			providerName: "secondary",
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentText, Text: "from secondary"},
					},
				}, nil
			},
		}

		fb, err := fallback.New(primary, secondary)
		require.NoError(t, err)

		var transformCalled bool
		mw := Middleware{
			TransformParams: func(_ context.Context, input TransformParamsInput) (provider.CallOptions, error) {
				transformCalled = true
				input.Params.Temperature = ptr(0.5)
				return input.Params, nil
			},
		}

		wrapped := WrapLanguageModel(fb, mw)

		result, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		assert.True(t, transformCalled, "middleware TransformParams should be called")
		require.Len(t, result.Content, 1)
		assert.Equal(t, "from secondary", result.Content[0].Text)
	})

	t.Run("FallbackWrappingMiddlewareModel", func(t *testing.T) {
		baseModel := &mockModel{
			providerName: "base",
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, errors.New("base failed")
			},
		}
		backupModel := &mockModel{
			providerName: "backup",
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentText, Text: "backup response"},
					},
				}, nil
			},
		}

		mw := DefaultSettings(DefaultSettingsOptions{
			Temperature: ptr(0.7),
		})

		wrappedPrimary := WrapLanguageModel(baseModel, mw)
		wrappedBackup := WrapLanguageModel(backupModel, mw)
		fb, err := fallback.New(wrappedPrimary, wrappedBackup)
		require.NoError(t, err)

		result, err := fb.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		require.Len(t, result.Content, 1)
		assert.Equal(t, "backup response", result.Content[0].Text)
	})

	t.Run("MultipleMiddlewaresWithFallback_Stream", func(t *testing.T) {
		ch := make(chan provider.StreamPart, 3)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "0"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "0", Delta: "<think>reason</think>answer"}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "0"}
		close(ch)

		model := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		defaults := DefaultSettings(DefaultSettingsOptions{Temperature: ptr(0.5)})
		reasoning := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})

		wrapped := WrapLanguageModel(model, defaults, reasoning)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var reasoningText, textContent string
		for p := range result.Stream {
			switch p.Type {
			case provider.PartReasoningDelta:
				reasoningText += p.Delta
			case provider.PartTextDelta:
				textContent += p.Delta
			}
		}
		assert.Equal(t, "reason", reasoningText)
		assert.Equal(t, "answer", textContent)
	})
}
