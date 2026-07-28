package middleware

import (
	"context"
	"fmt"
	"regexp"

	"github.com/grafana/ai-sdk/provider"
)

// CallType indicates whether the current model invocation is a generate or stream call.
type CallType string

const (
	CallTypeGenerate CallType = "generate"
	CallTypeStream   CallType = "stream"
)

// TransformParamsInput is passed to a Middleware's TransformParams hook.
type TransformParamsInput struct {
	Type   CallType
	Params provider.CallOptions
	Model  provider.LanguageModel
}

// WrapGenerateParams is passed to a Middleware's WrapGenerate hook.
type WrapGenerateParams struct {
	DoGenerate func(ctx context.Context) (*provider.GenerateResult, error)
	DoStream   func(ctx context.Context) (*provider.StreamResult, error)
	Params     provider.CallOptions
	Model      provider.LanguageModel
}

// WrapStreamParams is passed to a Middleware's WrapStream hook.
type WrapStreamParams struct {
	DoGenerate func(ctx context.Context) (*provider.GenerateResult, error)
	DoStream   func(ctx context.Context) (*provider.StreamResult, error)
	Params     provider.CallOptions
	Model      provider.LanguageModel
}

// Middleware defines optional hooks for intercepting and modifying language
// model calls. All function fields are optional; nil hooks pass through
// to the inner model unmodified.
type Middleware struct {
	TransformParams       func(ctx context.Context, params TransformParamsInput) (provider.CallOptions, error)
	WrapGenerate          func(ctx context.Context, params WrapGenerateParams) (*provider.GenerateResult, error)
	WrapStream            func(ctx context.Context, params WrapStreamParams) (*provider.StreamResult, error)
	OverrideProvider      func(model provider.LanguageModel) string
	OverrideModelID       func(model provider.LanguageModel) string
	OverrideSupportedURLs func(model provider.LanguageModel) map[string][]*regexp.Regexp
}

// WrapLanguageModel wraps a LanguageModel with one or more middlewares,
// returning a new LanguageModel. The first middleware in the list is the
// outermost wrapper (processes input first). Each layer returns a new
// LanguageModel; the original model is never mutated.
func WrapLanguageModel(model provider.LanguageModel, middlewares ...Middleware) provider.LanguageModel {
	return wrapLanguageModel(model, middlewares, WrapOptions{})
}

// WrapOptions configures optional overrides applied by Wrap.
// ModelID and ProviderID take highest precedence, above any
// middleware-level OverrideModelID/OverrideProvider hooks.
type WrapOptions struct {
	Model      provider.LanguageModel
	Middleware []Middleware
	ModelID    string
	ProviderID string
}

// Wrap wraps a LanguageModel with middlewares and optional top-level
// overrides. This is the full-featured variant of WrapLanguageModel,
// matching the upstream wrapLanguageModel({ model, middleware, modelId, providerId }).
func Wrap(opts WrapOptions) provider.LanguageModel {
	return wrapLanguageModel(opts.Model, opts.Middleware, opts)
}

func wrapLanguageModel(model provider.LanguageModel, middlewares []Middleware, opts WrapOptions) provider.LanguageModel {
	hasOverrides := opts.ModelID != "" || opts.ProviderID != ""
	if len(middlewares) == 0 && !hasOverrides {
		return model
	}

	mws := middlewares
	if len(mws) == 0 && hasOverrides {
		mws = []Middleware{{}}
	}

	wrapped := model
	for i := len(mws) - 1; i >= 0; i-- {
		wrapped = &wrappedModel{
			inner:      wrapped,
			mw:         mws[i],
			modelID:    opts.ModelID,
			providerID: opts.ProviderID,
		}
	}
	return wrapped
}

type wrappedModel struct {
	inner      provider.LanguageModel
	mw         Middleware
	modelID    string
	providerID string
}

func (w *wrappedModel) SpecificationVersion() string { return w.inner.SpecificationVersion() }

func (w *wrappedModel) Provider() string {
	if w.providerID != "" {
		return w.providerID
	}
	if w.mw.OverrideProvider != nil {
		return w.mw.OverrideProvider(w.inner)
	}
	return w.inner.Provider()
}

func (w *wrappedModel) ModelID() string {
	if w.modelID != "" {
		return w.modelID
	}
	if w.mw.OverrideModelID != nil {
		return w.mw.OverrideModelID(w.inner)
	}
	return w.inner.ModelID()
}

func (w *wrappedModel) SupportedURLs() map[string][]*regexp.Regexp {
	if w.mw.OverrideSupportedURLs != nil {
		return w.mw.OverrideSupportedURLs(w.inner)
	}
	return w.inner.SupportedURLs()
}

func (w *wrappedModel) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	transformedParams, err := w.transformParams(ctx, CallTypeGenerate, params)
	if err != nil {
		return nil, fmt.Errorf("middleware transform params: %w", err)
	}

	doGenerate := func(ctx context.Context) (*provider.GenerateResult, error) {
		return w.inner.DoGenerate(ctx, transformedParams)
	}
	doStream := func(ctx context.Context) (*provider.StreamResult, error) {
		return w.inner.DoStream(ctx, transformedParams)
	}

	if w.mw.WrapGenerate != nil {
		return w.mw.WrapGenerate(ctx, WrapGenerateParams{
			DoGenerate: doGenerate,
			DoStream:   doStream,
			Params:     transformedParams,
			Model:      w.inner,
		})
	}
	return doGenerate(ctx)
}

func (w *wrappedModel) DoStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	transformedParams, err := w.transformParams(ctx, CallTypeStream, params)
	if err != nil {
		return nil, fmt.Errorf("middleware transform params: %w", err)
	}

	doGenerate := func(ctx context.Context) (*provider.GenerateResult, error) {
		return w.inner.DoGenerate(ctx, transformedParams)
	}
	doStream := func(ctx context.Context) (*provider.StreamResult, error) {
		return w.inner.DoStream(ctx, transformedParams)
	}

	if w.mw.WrapStream != nil {
		return w.mw.WrapStream(ctx, WrapStreamParams{
			DoGenerate: doGenerate,
			DoStream:   doStream,
			Params:     transformedParams,
			Model:      w.inner,
		})
	}
	return doStream(ctx)
}

func (w *wrappedModel) transformParams(ctx context.Context, callType CallType, params provider.CallOptions) (provider.CallOptions, error) {
	if w.mw.TransformParams == nil {
		return params, nil
	}
	return w.mw.TransformParams(ctx, TransformParamsInput{
		Type:   callType,
		Params: params,
		Model:  w.inner,
	})
}
