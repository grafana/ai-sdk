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
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
)

func NewConfiguredSelector(resolver catalog.ModelResolver) execution.RequestSelector {
	selectModel := execution.CatalogSelector(resolver)
	return func(ctx context.Context, id string, options provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
		if !hasAccountAccess(ctx, gatewayauth.ConfiguredAccounts) {
			return execution.Selection{}, execution.ErrAccountAccess
		}
		return selectModel(ctx, id, options, gateway)
	}
}

func NewBYOKSelector(client *http.Client, factory ModelFactory) execution.RequestSelector {
	return func(ctx context.Context, id string, _ provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
		if !hasAccountAccess(ctx, gatewayauth.RequestBYOK) {
			return execution.Selection{}, execution.ErrAccountAccess
		}
		request, err := byok.DecodeRequest(id, gateway, nil)
		switch {
		case errors.Is(err, byok.ErrInvalidSelector):
			return execution.Selection{}, execution.ErrInvalidBYOKSelector
		case errors.Is(err, byok.ErrInvalidRequest):
			return execution.Selection{}, execution.ErrInvalidBYOK
		case err != nil:
			return execution.Selection{}, err
		}
		lower, err := byok.New(request.Provider, request.Model, request.Accounts, client)
		if err != nil {
			return execution.Selection{}, err
		}
		model, err := factory(id, lower)
		if err != nil {
			return execution.Selection{}, err
		}
		return execution.Selection{ID: id, Model: model}, nil
	}
}

func ConfiguredDiscovery(next http.Handler, writer *providerv4.HostErrorWriter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hasAccountAccess(r.Context(), gatewayauth.ConfiguredAccounts) {
			writer.Write(w, providerv4.HostErrorPermission)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func BYOKDiscovery(writer *providerv4.HostErrorWriter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hasAccountAccess(r.Context(), gatewayauth.RequestBYOK) {
			writer.Write(w, providerv4.HostErrorPermission)
			return
		}
		writer.Write(w, providerv4.HostErrorBYOKDiscovery)
	})
}

func hasAccountAccess(ctx context.Context, access gatewayauth.AccountAccess) bool {
	caller, ok := gatewayauth.CallerFromContext(ctx)
	return ok && caller.AccountAccess() == access
}
