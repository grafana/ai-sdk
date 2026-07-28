package middleware

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ provider.LanguageModel = (*wrappedModel)(nil)
	_ provider.LanguageModel = (*mockModel)(nil)
)

type mockModel struct {
	providerName string
	modelID      string
	doGenerate   func(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error)
	doStream     func(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error)
}

func (m *mockModel) SpecificationVersion() string               { return "v4" }
func (m *mockModel) Provider() string                           { return m.providerName }
func (m *mockModel) ModelID() string                            { return m.modelID }
func (m *mockModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }

func (m *mockModel) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	if m.doGenerate != nil {
		return m.doGenerate(ctx, params)
	}
	return &provider.GenerateResult{}, nil
}

func (m *mockModel) DoStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	if m.doStream != nil {
		return m.doStream(ctx, params)
	}
	ch := make(chan provider.StreamPart)
	close(ch)
	return &provider.StreamResult{Stream: ch}, nil
}

func ptr[T any](v T) *T { return &v }

func TestWrapLanguageModel(t *testing.T) {
	t.Run("NoMiddlewares_ReturnsSameModel", func(t *testing.T) {
		model := &mockModel{providerName: "test", modelID: "m1"}
		wrapped := WrapLanguageModel(model)
		assert.Equal(t, model, wrapped)
	})

	t.Run("NilHooks_Passthrough", func(t *testing.T) {
		result := &provider.GenerateResult{
			FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		}
		model := &mockModel{
			providerName: "test",
			modelID:      "m1",
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return result, nil
			},
		}

		wrapped := WrapLanguageModel(model, Middleware{})
		got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		assert.Equal(t, result, got)
	})

	t.Run("TransformParams_ModifiesCallOptions", func(t *testing.T) {
		var receivedParams provider.CallOptions
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				receivedParams = params
				return &provider.GenerateResult{}, nil
			},
		}

		mw := Middleware{
			TransformParams: func(_ context.Context, input TransformParamsInput) (provider.CallOptions, error) {
				input.Params.Temperature = ptr(0.5)
				return input.Params, nil
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		require.NotNil(t, receivedParams.Temperature)
		assert.Equal(t, 0.5, *receivedParams.Temperature)
	})

	t.Run("TransformParams_ReceivesCallType", func(t *testing.T) {
		var generateType, streamType CallType
		model := &mockModel{}
		mw := Middleware{
			TransformParams: func(_ context.Context, input TransformParamsInput) (provider.CallOptions, error) {
				if input.Type == CallTypeGenerate {
					generateType = input.Type
				} else {
					streamType = input.Type
				}
				return input.Params, nil
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		_, _ = wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		_, _ = wrapped.DoStream(context.Background(), provider.CallOptions{})

		assert.Equal(t, CallTypeGenerate, generateType)
		assert.Equal(t, CallTypeStream, streamType)
	})

	t.Run("WrapGenerate_InterceptsAndDelegates", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "original"}},
				}, nil
			},
		}

		mw := Middleware{
			WrapGenerate: func(ctx context.Context, params WrapGenerateParams) (*provider.GenerateResult, error) {
				result, err := params.DoGenerate(ctx)
				if err != nil {
					return nil, err
				}
				result.Content = append(result.Content, provider.GenerateContentPart{Type: provider.ContentText, Text: "added"})
				return result, nil
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		require.Len(t, got.Content, 2)
		assert.Equal(t, "original", got.Content[0].Text)
		assert.Equal(t, "added", got.Content[1].Text)
	})

	t.Run("WrapStream_InterceptsAndDelegates", func(t *testing.T) {
		ch := make(chan provider.StreamPart, 2)
		ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "hello"}
		ch <- provider.StreamPart{Type: provider.PartFinish}
		close(ch)

		model := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		var wrappedCalled bool
		mw := Middleware{
			WrapStream: func(ctx context.Context, params WrapStreamParams) (*provider.StreamResult, error) {
				wrappedCalled = true
				return params.DoStream(ctx)
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		assert.True(t, wrappedCalled)

		var parts []provider.StreamPart
		for p := range result.Stream {
			parts = append(parts, p)
		}
		require.Len(t, parts, 2)
		assert.Equal(t, "hello", parts[0].Delta)
	})

	t.Run("CrossMode_WrapGenerateCallsDoStream", func(t *testing.T) {
		ch := make(chan provider.StreamPart, 1)
		ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "streamed"}
		close(ch)

		model := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		mw := Middleware{
			WrapGenerate: func(ctx context.Context, params WrapGenerateParams) (*provider.GenerateResult, error) {
				streamResult, err := params.DoStream(ctx)
				if err != nil {
					return nil, err
				}
				var text string
				for p := range streamResult.Stream {
					if p.Type == provider.PartTextDelta {
						text += p.Delta
					}
				}
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: text}},
				}, nil
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		require.Len(t, got.Content, 1)
		assert.Equal(t, "streamed", got.Content[0].Text)
	})

	t.Run("CrossMode_WrapStreamCallsDoGenerate", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "generated"}},
				}, nil
			},
		}

		mw := Middleware{
			WrapStream: func(ctx context.Context, params WrapStreamParams) (*provider.StreamResult, error) {
				result, err := params.DoGenerate(ctx)
				if err != nil {
					return nil, err
				}
				ch := make(chan provider.StreamPart, 1)
				ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: result.Content[0].Text}
				close(ch)
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var parts []provider.StreamPart
		for p := range result.Stream {
			parts = append(parts, p)
		}
		require.Len(t, parts, 1)
		assert.Equal(t, "generated", parts[0].Delta)
	})

	t.Run("ErrorPropagation_TransformParams", func(t *testing.T) {
		model := &mockModel{}
		mw := Middleware{
			TransformParams: func(_ context.Context, _ TransformParamsInput) (provider.CallOptions, error) {
				return provider.CallOptions{}, errors.New("transform error")
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transform error")

		_, err = wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transform error")
	})

	t.Run("ErrorPropagation_WrapGenerate", func(t *testing.T) {
		model := &mockModel{}
		mw := Middleware{
			WrapGenerate: func(_ context.Context, _ WrapGenerateParams) (*provider.GenerateResult, error) {
				return nil, errors.New("wrap generate error")
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.Equal(t, "wrap generate error", err.Error())
	})

	t.Run("ErrorPropagation_WrapStream", func(t *testing.T) {
		model := &mockModel{}
		mw := Middleware{
			WrapStream: func(_ context.Context, _ WrapStreamParams) (*provider.StreamResult, error) {
				return nil, errors.New("wrap stream error")
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.Equal(t, "wrap stream error", err.Error())
	})

	t.Run("MetadataOverride", func(t *testing.T) {
		model := &mockModel{providerName: "original-provider", modelID: "original-model"}
		mw := Middleware{
			OverrideProvider: func(_ provider.LanguageModel) string { return "custom-provider" },
			OverrideModelID:  func(_ provider.LanguageModel) string { return "custom-model" },
		}

		wrapped := WrapLanguageModel(model, mw)
		assert.Equal(t, "custom-provider", wrapped.Provider())
		assert.Equal(t, "custom-model", wrapped.ModelID())
		assert.Equal(t, "v4", wrapped.SpecificationVersion())
		assert.Nil(t, wrapped.SupportedURLs())
	})

	t.Run("OverrideSupportedURLs", func(t *testing.T) {
		model := &mockModel{providerName: "test", modelID: "m1"}
		customURLs := map[string][]*regexp.Regexp{
			"image/*": {regexp.MustCompile(`https://example\.com/.*`)},
		}
		mw := Middleware{
			OverrideSupportedURLs: func(_ provider.LanguageModel) map[string][]*regexp.Regexp {
				return customURLs
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		assert.Equal(t, customURLs, wrapped.SupportedURLs())
	})

	t.Run("SupportedURLs_DelegatesWhenNoOverride", func(t *testing.T) {
		model := &mockModel{providerName: "test", modelID: "m1"}
		wrapped := WrapLanguageModel(model, Middleware{})
		assert.Nil(t, wrapped.SupportedURLs())
	})

	t.Run("MetadataOverride_ReceivesInnerModel", func(t *testing.T) {
		model := &mockModel{providerName: "inner-provider", modelID: "inner-model"}
		mw := Middleware{
			OverrideProvider: func(m provider.LanguageModel) string { return "wrapped-" + m.Provider() },
			OverrideModelID:  func(m provider.LanguageModel) string { return "wrapped-" + m.ModelID() },
		}

		wrapped := WrapLanguageModel(model, mw)
		assert.Equal(t, "wrapped-inner-provider", wrapped.Provider())
		assert.Equal(t, "wrapped-inner-model", wrapped.ModelID())
	})

	t.Run("MultipleMiddlewares_CompositionOrder", func(t *testing.T) {
		var order []string
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				order = append(order, "model")
				return &provider.GenerateResult{}, nil
			},
		}

		mkMiddleware := func(name string) Middleware {
			return Middleware{
				TransformParams: func(_ context.Context, input TransformParamsInput) (provider.CallOptions, error) {
					order = append(order, name+"-transform")
					return input.Params, nil
				},
				WrapGenerate: func(ctx context.Context, params WrapGenerateParams) (*provider.GenerateResult, error) {
					order = append(order, name+"-wrap-before")
					result, err := params.DoGenerate(ctx)
					order = append(order, name+"-wrap-after")
					return result, err
				},
			}
		}

		wrapped := WrapLanguageModel(model, mkMiddleware("A"), mkMiddleware("B"), mkMiddleware("C"))
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		assert.Equal(t, []string{
			"A-transform",
			"A-wrap-before",
			"B-transform",
			"B-wrap-before",
			"C-transform",
			"C-wrap-before",
			"model",
			"C-wrap-after",
			"B-wrap-after",
			"A-wrap-after",
		}, order)
	})

	t.Run("MultipleMiddlewares_CompositionOrder_Stream", func(t *testing.T) {
		var order []string
		ch := make(chan provider.StreamPart)
		close(ch)
		model := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				order = append(order, "model")
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		mkMiddleware := func(name string) Middleware {
			return Middleware{
				TransformParams: func(_ context.Context, input TransformParamsInput) (provider.CallOptions, error) {
					order = append(order, name+"-transform")
					return input.Params, nil
				},
				WrapStream: func(ctx context.Context, params WrapStreamParams) (*provider.StreamResult, error) {
					order = append(order, name+"-wrap-before")
					result, err := params.DoStream(ctx)
					order = append(order, name+"-wrap-after")
					return result, err
				},
			}
		}

		wrapped := WrapLanguageModel(model, mkMiddleware("A"), mkMiddleware("B"))
		_, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		assert.Equal(t, []string{
			"A-transform",
			"A-wrap-before",
			"B-transform",
			"B-wrap-before",
			"model",
			"B-wrap-after",
			"A-wrap-after",
		}, order)
	})

	t.Run("TransformParams_TransformedParamsReachWrapHook", func(t *testing.T) {
		model := &mockModel{}
		mw := Middleware{
			TransformParams: func(_ context.Context, input TransformParamsInput) (provider.CallOptions, error) {
				input.Params.Temperature = ptr(0.9)
				return input.Params, nil
			},
			WrapGenerate: func(ctx context.Context, params WrapGenerateParams) (*provider.GenerateResult, error) {
				require.NotNil(t, params.Params.Temperature)
				assert.Equal(t, 0.9, *params.Params.Temperature)
				return params.DoGenerate(ctx)
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
	})

	t.Run("ContextPropagation_WrapGenerate", func(t *testing.T) {
		type ctxKey struct{}
		model := &mockModel{
			doGenerate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				val := ctx.Value(ctxKey{})
				require.NotNil(t, val)
				assert.Equal(t, "injected", val.(string))
				return &provider.GenerateResult{}, nil
			},
		}

		mw := Middleware{
			WrapGenerate: func(ctx context.Context, params WrapGenerateParams) (*provider.GenerateResult, error) {
				ctx = context.WithValue(ctx, ctxKey{}, "injected")
				return params.DoGenerate(ctx)
			},
		}

		wrapped := WrapLanguageModel(model, mw)
		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
	})
}

func TestWrap(t *testing.T) {
	t.Run("ModelID_TakesPrecedenceOverMiddleware", func(t *testing.T) {
		model := &mockModel{providerName: "original", modelID: "original-model"}
		mw := Middleware{
			OverrideModelID: func(_ provider.LanguageModel) string { return "middleware-model" },
		}

		wrapped := Wrap(WrapOptions{
			Model:      model,
			Middleware: []Middleware{mw},
			ModelID:    "top-level-model",
		})

		assert.Equal(t, "top-level-model", wrapped.ModelID())
	})

	t.Run("ProviderID_TakesPrecedenceOverMiddleware", func(t *testing.T) {
		model := &mockModel{providerName: "original", modelID: "m1"}
		mw := Middleware{
			OverrideProvider: func(_ provider.LanguageModel) string { return "middleware-provider" },
		}

		wrapped := Wrap(WrapOptions{
			Model:      model,
			Middleware: []Middleware{mw},
			ProviderID: "top-level-provider",
		})

		assert.Equal(t, "top-level-provider", wrapped.Provider())
	})

	t.Run("NoOverrides_DelegatesToMiddleware", func(t *testing.T) {
		model := &mockModel{providerName: "original", modelID: "m1"}
		mw := Middleware{
			OverrideModelID: func(_ provider.LanguageModel) string { return "mw-model" },
		}

		wrapped := Wrap(WrapOptions{
			Model:      model,
			Middleware: []Middleware{mw},
		})

		assert.Equal(t, "mw-model", wrapped.ModelID())
		assert.Equal(t, "original", wrapped.Provider())
	})

	t.Run("PartialOverride_OnlyModelID", func(t *testing.T) {
		model := &mockModel{providerName: "original", modelID: "m1"}

		wrapped := Wrap(WrapOptions{
			Model:   model,
			ModelID: "custom-model",
		})

		assert.Equal(t, "custom-model", wrapped.ModelID())
		assert.Equal(t, "original", wrapped.Provider())
	})

	t.Run("MiddlewareHooksSeesOverriddenModelID", func(t *testing.T) {
		model := &mockModel{providerName: "original", modelID: "original-model"}
		var innerModelID, outerModelID string

		inner := Middleware{
			WrapGenerate: func(ctx context.Context, params WrapGenerateParams) (*provider.GenerateResult, error) {
				innerModelID = params.Model.ModelID()
				return params.DoGenerate(ctx)
			},
		}
		outer := Middleware{
			WrapGenerate: func(ctx context.Context, params WrapGenerateParams) (*provider.GenerateResult, error) {
				outerModelID = params.Model.ModelID()
				return params.DoGenerate(ctx)
			},
		}

		wrapped := Wrap(WrapOptions{
			Model:      model,
			Middleware: []Middleware{outer, inner},
			ModelID:    "top-level",
		})

		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		assert.Equal(t, "original-model", innerModelID, "innermost middleware sees original model")
		assert.Equal(t, "top-level", outerModelID, "outer middleware sees overridden modelID on inner wrapped model")
	})

	t.Run("MiddlewareStillApplied", func(t *testing.T) {
		var transformCalled bool
		model := &mockModel{}
		mw := Middleware{
			TransformParams: func(_ context.Context, input TransformParamsInput) (provider.CallOptions, error) {
				transformCalled = true
				return input.Params, nil
			},
		}

		wrapped := Wrap(WrapOptions{
			Model:      model,
			Middleware: []Middleware{mw},
			ModelID:    "custom",
		})

		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		assert.True(t, transformCalled)
	})
}
