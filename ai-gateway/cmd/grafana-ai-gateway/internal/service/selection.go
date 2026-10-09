package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/byok"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
)

func NewConfiguredSelector(resolver catalog.ModelResolver) providerv4.RequestSelector {
	selectModel := providerv4.CatalogSelector(resolver)
	return func(ctx context.Context, id string, options provider.CallOptions, gateway json.RawMessage) (providerv4.Selection, error) {
		if !hasAccountAccess(ctx, gatewayauth.ConfiguredAccounts) {
			return providerv4.Selection{}, providerv4.ErrAccountAccess
		}
		return selectModel(ctx, id, options, gateway)
	}
}

func NewBYOKSelector(client *http.Client, factory ModelFactory) providerv4.RequestSelector {
	return func(ctx context.Context, id string, _ provider.CallOptions, gateway json.RawMessage) (providerv4.Selection, error) {
		if !hasAccountAccess(ctx, gatewayauth.RequestBYOK) {
			return providerv4.Selection{}, providerv4.ErrAccountAccess
		}
		request, err := byok.DecodeRequest(id, gateway, nil)
		switch {
		case errors.Is(err, byok.ErrInvalidSelector):
			return providerv4.Selection{}, providerv4.ErrInvalidBYOKSelector
		case errors.Is(err, byok.ErrInvalidRequest):
			return providerv4.Selection{}, providerv4.ErrInvalidBYOK
		case err != nil:
			return providerv4.Selection{}, err
		}
		lower, err := byok.New(request.Provider, request.Model, request.Accounts, client)
		if err != nil {
			return providerv4.Selection{}, err
		}
		model, err := factory(id, lower)
		if err != nil {
			return providerv4.Selection{}, err
		}
		return providerv4.Selection{ID: id, Model: model}, nil
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
