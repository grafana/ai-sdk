package v4

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

type Selection struct {
	ID         string
	Model      provider.LanguageModel
	configured *selectionExecution
}

type selectionExecution struct {
	candidates []catalog.ConfiguredCandidate
}

type RequestSelector func(context.Context, string, provider.CallOptions, json.RawMessage) (Selection, error)

var (
	ErrReservedProviderOptions   = errors.New("providerwire v4: reserved provider options")
	ErrInvalidBYOK               = errors.New("providerwire v4: invalid BYOK credentials")
	ErrInvalidBYOKSelector       = errors.New("providerwire v4: invalid BYOK model selector")
	ErrUnsupportedGatewayControl = errors.New("providerwire v4: unsupported gateway control")
)

func CatalogSelector(resolver catalog.ModelResolver) RequestSelector {
	if isNilInterface(resolver) {
		return nil
	}
	return func(ctx context.Context, modelID string, _ provider.CallOptions, gateway json.RawMessage) (Selection, error) {
		if gateway != nil {
			return Selection{}, ErrReservedProviderOptions
		}
		resolved, err := resolver.ResolveModel(ctx, modelID)
		if err != nil {
			return Selection{}, err
		}
		return Selection{
			ID:    resolved.ID,
			Model: resolved.Model,
			configured: &selectionExecution{
				candidates: slices.Clone(resolved.Candidates),
			},
		}, nil
	}
}

func (h *handler) selectModel(ctx context.Context, modelID string, options provider.CallOptions, gateway json.RawMessage) (Selection, error) {
	if err := ctx.Err(); err != nil {
		return Selection{}, err
	}
	type outcome struct {
		selection Selection
		err       error
	}
	ready := make(chan outcome, 1)
	go func() {
		result := outcome{}
		defer func() {
			if recover() != nil {
				result = outcome{err: errRuntimeInternal}
			}
			ready <- result
		}()
		result.selection, result.err = h.selector(ctx, modelID, options, gateway)
		if result.err == nil && !validSelection(result.selection) {
			result = outcome{err: errRuntimeInternal}
		}
	}()
	select {
	case result := <-ready:
		if err := ctx.Err(); err != nil {
			return Selection{}, err
		}
		return result.selection, result.err
	case <-ctx.Done():
		return Selection{}, ctx.Err()
	}
}
