package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/byok"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/routing"
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

// ConfiguredModels resolves configured models and composes a request's routed
// chain over their candidates. [Catalog] implements it.
type ConfiguredModels interface {
	catalog.ModelResolver
	selectRouted(ctx context.Context, id string, controls routing.Controls) (execution.Selection, error)
}

// NewSelector returns the selector for every access mode. Configured callers
// resolve through models, and their routing controls compose a request chain
// over configured candidates. BYOK callers, when byokOptions is set, build a
// model from the request accounts and routing controls. Every other caller
// fails with [execution.ErrAccountAccess]. It returns nil for nil models.
func NewSelector(models ConfiguredModels, byokOptions *BYOKOptions) execution.RequestSelector {
	selectConfigured := execution.CatalogSelector(models)
	if selectConfigured == nil {
		return nil
	}
	return func(ctx context.Context, id string, options provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
		caller, _ := gatewayauth.CallerFromContext(ctx)
		switch {
		case caller.AccountAccess() == gatewayauth.ConfiguredAccounts:
			controls, err := routing.Decode(gateway, routing.ConfiguredAccess)
			if err != nil {
				return execution.Selection{}, err
			}
			if controls.Routed() {
				return models.selectRouted(ctx, id, controls)
			}
			return selectConfigured(ctx, id, options, nil)
		case caller.AccountAccess() == gatewayauth.RequestBYOK && byokOptions != nil:
			controls, err := routing.Decode(gateway, routing.RequestAccountAccess)
			if err != nil {
				return execution.Selection{}, err
			}
			return selectBYOK(id, controls, *byokOptions)
		default:
			return execution.Selection{}, execution.ErrAccountAccess
		}
	}
}

func selectBYOK(id string, controls routing.Controls, options BYOKOptions) (execution.Selection, error) {
	request, err := byok.DecodeRequest(id, controls, nil)
	switch {
	case errors.Is(err, byok.ErrInvalidSelector):
		return execution.Selection{}, execution.ErrInvalidBYOKSelector
	case errors.Is(err, byok.ErrInvalidRequest):
		return execution.Selection{}, execution.ErrInvalidBYOK
	case err != nil:
		return execution.Selection{}, err
	}
	requested := request.Destinations[0]
	lower, err := byok.New(requested.Provider, requested.Model, requested.Accounts, options.Client)
	if err != nil {
		return execution.Selection{}, err
	}
	model, err := options.Factory(id, lower)
	if err != nil {
		return execution.Selection{}, err
	}
	return execution.AccountSelection(id, model, []execution.RequestAccounts{{
		Provider: string(requested.Provider),
		ModelID:  requested.Model,
		Count:    len(requested.Accounts),
	}}), nil
}
