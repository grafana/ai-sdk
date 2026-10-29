package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/byok"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

// BYOKOptions enables request-scoped accounts for BYOK callers.
type BYOKOptions struct {
	// Client is the shared model HTTP client for request accounts.
	Client *http.Client
	// Factory composes the logical model around the request accounts.
	Factory ModelFactory
}

// NewSelector returns the selector for every access mode. Configured callers
// resolve through resolver and cannot send gateway controls. BYOK callers,
// when byokOptions is set, build a model from the request accounts in those
// controls. Every other caller fails with [execution.ErrAccountAccess]. It
// returns nil for a nil resolver.
func NewSelector(resolver catalog.ModelResolver, byokOptions *BYOKOptions) execution.RequestSelector {
	selectConfigured := execution.CatalogSelector(resolver)
	if selectConfigured == nil {
		return nil
	}
	return func(ctx context.Context, id string, options provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
		caller, _ := gatewayauth.CallerFromContext(ctx)
		switch {
		case caller.AccountAccess() == gatewayauth.ConfiguredAccounts:
			return selectConfigured(ctx, id, options, gateway)
		case caller.AccountAccess() == gatewayauth.RequestBYOK && byokOptions != nil:
			return selectBYOK(id, gateway, *byokOptions)
		default:
			return execution.Selection{}, execution.ErrAccountAccess
		}
	}
}

func selectBYOK(id string, gateway json.RawMessage, options BYOKOptions) (execution.Selection, error) {
	request, err := byok.DecodeRequest(id, gateway, nil)
	switch {
	case errors.Is(err, byok.ErrInvalidSelector):
		return execution.Selection{}, execution.ErrInvalidBYOKSelector
	case errors.Is(err, byok.ErrInvalidRequest):
		return execution.Selection{}, execution.ErrInvalidBYOK
	case err != nil:
		return execution.Selection{}, err
	}
	lower, err := byok.New(request.Provider, request.Model, request.Accounts, options.Client)
	if err != nil {
		return execution.Selection{}, err
	}
	model, err := options.Factory(id, lower)
	if err != nil {
		return execution.Selection{}, err
	}
	return execution.AccountSelection(id, model, execution.RequestAccounts{
		Provider: string(request.Provider),
		ModelID:  request.Model,
		Count:    len(request.Accounts),
	}), nil
}
