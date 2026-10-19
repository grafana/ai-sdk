package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/resource"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

const (
	testInternalService = "internal-service"
	testNamespace       = "stacks-1"
)

type verifiedInfo struct {
	types.AuthInfo
	extra map[string][]string
}

func (info verifiedInfo) GetExtra() map[string][]string { return info.extra }

func verifiedContext(ctx context.Context, service, identity, namespace string) context.Context {
	extra := map[string][]string{}
	if service != "" {
		extra[gatewayauth.DirectCallerServiceKey] = []string{service}
	}
	if identity != "" {
		extra[gatewayauth.IdentityActorKey] = []string{identity}
	}
	ctx = types.WithAuthInfo(ctx, verifiedInfo{extra: extra})
	return resource.WithRouteRequestInfo(ctx, &resource.RouteRequestInfo{FullIdentifier: resource.FullIdentifier{Namespace: namespace}})
}

func verifiedCaller(t *testing.T, mode gatewayauth.AccessMode, internal []string, service, identity, namespace string) (*gatewayauth.Caller, gatewayauth.Decision) {
	t.Helper()
	policy := gatewayauth.AccessPolicy{Mode: mode, InternalServices: internal}
	info, _ := types.AuthInfoFrom(verifiedContext(context.Background(), service, identity, namespace))
	caller, decision := policy.Decide(info, namespace)
	if decision == gatewayauth.DecisionRefused {
		return nil, decision
	}
	return &caller, decision
}

func withAccess(t *testing.T, mode gatewayauth.AccessMode, access gatewayauth.AccountAccess, next http.Handler) (http.Handler, http.Header) {
	t.Helper()
	service, identity := testInternalService, ""
	switch {
	case access == gatewayauth.RequestBYOK:
		require.Equal(t, gatewayauth.AccessModeCloud, mode)
		service, identity = "front-door", "user:u1"
	case mode == gatewayauth.AccessModeSelfHosted:
		service, identity = "", "service-account:sa1"
	}
	policy := gatewayauth.AccessPolicy{Mode: mode}
	if mode == gatewayauth.AccessModeCloud {
		policy.InternalServices = []string{testInternalService}
	}
	writer := providerv4.NewHostErrorWriter()
	protected := gatewayauth.AccessMiddleware(policy, func(w http.ResponseWriter) { writer.Write(w, providerv4.HostErrorPermission) }, func(context.Context, gatewayauth.Decision, *gatewayauth.Caller) {}, next)
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		protected.ServeHTTP(w, request.WithContext(verifiedContext(request.Context(), service, identity, testNamespace)))
	}), http.Header{}
}

func fixedRoute(name string) func(*http.Request) string {
	return func(*http.Request) string { return name }
}
