package enrichment

import (
	"context"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

// Middleware returns middleware that enriches provider call headers and provider options.
func Middleware(opts Options) middleware.Middleware {
	normalized := normalizeOptions(opts)
	return middleware.Middleware{
		TransformParams: func(ctx context.Context, input middleware.TransformParamsInput) (provider.CallOptions, error) {
			return transformParams(ctx, normalized, input)
		},
	}
}

// Wrap wraps base with enrichment middleware.
func Wrap(base provider.LanguageModel, opts Options) provider.LanguageModel {
	return middleware.WrapLanguageModel(base, Middleware(opts))
}

func transformParams(ctx context.Context, opts normalizedOptions, input middleware.TransformParamsInput) (provider.CallOptions, error) {
	current := input.Params
	values, err := collectValues(ctx, opts, CallInput{
		Type:   input.Type,
		Params: input.Params,
		Model:  input.Model,
	})
	if err != nil {
		return handleError(ctx, opts, current, err)
	}

	normalizedValues := normalizeValues(ctx, values, opts)
	include := stringSet(opts.filter.Include)
	exclude := stringSet(opts.filter.Exclude)

	if headerOutputEnabled(opts.headers) {
		headerSpecific := headerSelection(opts.headers)
		headerValues := selectedValues(normalizedValues, include, headerSpecific, exclude)
		current, err = applyHeaders(current, headerValues, include, opts.headers)
		if err != nil {
			return handleError(ctx, opts, current, err)
		}
	}

	if providerOptionsOutputEnabled(opts.providerOptions) {
		providerSpecific := providerOptionsSelection(opts.providerOptions)
		providerValues := selectedValues(normalizedValues, include, providerSpecific, exclude)
		current, err = applyProviderOptions(current, providerValues, opts.providerOptions)
		if err != nil {
			return handleError(ctx, opts, current, err)
		}
	}

	return current, nil
}

func handleError(ctx context.Context, opts normalizedOptions, current provider.CallOptions, err error) (provider.CallOptions, error) {
	if opts.onError == nil {
		return current, err
	}
	if replacement := opts.onError(ctx, err); replacement != nil {
		return current, replacement
	}
	return current, nil
}
