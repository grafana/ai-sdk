package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/require"
)

func TestCompletionScopeUsesVerifiedActorAndNamespace(t *testing.T) {
	_, ok := completionScope(context.Background())
	require.False(t, ok)
	for _, actor := range []types.IdentityType{types.TypeUser, types.TypeServiceAccount} {
		var claims authn.Claims[authn.AccessTokenClaims]
		claims.Rest.Namespace = "tenant-a"
		claims.Rest.Actor = &authn.ActorClaims{IDTokenClaims: authn.IDTokenClaims{Type: actor, Identifier: "same-id"}}
		info := authn.NewAccessTokenAuthInfo(claims)
		for _, namespace := range []string{"tenant-a", "tenant-b"} {
			reached := false
			handler := gatewayauth.AccessMiddleware(gatewayauth.AccessPolicy{Mode: gatewayauth.AccessModeSelfHosted}, func(w http.ResponseWriter) { w.WriteHeader(403) }, func(context.Context, gatewayauth.Caller) {}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				scope, ok := completionScope(r.Context())
				require.True(t, ok)
				require.Equal(t, "tenant-a", scope.Namespace)
				require.Equal(t, "identity:"+info.GetUID(), scope.Owner)
			}))
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("X-Scope-OrgID", "spoofed")
			req.Header.Set("X-Grafana-Id", "spoofed")
			ctx := types.WithAuthInfo(req.Context(), info)
			ctx = resource.WithRouteRequestInfo(ctx, &resource.RouteRequestInfo{FullIdentifier: resource.FullIdentifier{Namespace: namespace}})
			handler.ServeHTTP(httptest.NewRecorder(), req.WithContext(ctx))
			require.Equal(t, namespace == "tenant-a", reached)
		}
	}
}
