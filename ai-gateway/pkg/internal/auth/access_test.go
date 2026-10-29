package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/resource"

	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

func TestAccessPolicy_Decide(t *testing.T) {
	cloudHost := newHost(t, platformhost.TopologyCloud)
	selfHost := newHost(t, platformhost.TopologySelfHosted)
	cloud := chains{t: t, host: cloudHost}
	self := chains{t: t, host: selfHost}
	allowlisted := AccessPolicy{Mode: AccessModeCloud, InternalServices: []string{"svc-a"}}
	emptyAllowlist := AccessPolicy{Mode: AccessModeCloud}
	selfHosted := AccessPolicy{Mode: AccessModeSelfHosted}

	tests := []struct {
		name       string
		policy     AccessPolicy
		keys       *platformhost.Host
		token      string
		namespace  string
		access     AccountAccess
		identity   string
		attributed string
	}{
		{name: "cloud allowlisted direct service", policy: allowlisted, token: cloud.cloud(cloud.service("svc-a", testNamespace)), access: ConfiguredAccounts, attributed: "svc-a"},
		{name: "cloud allowlisted service for a user", policy: allowlisted, token: cloud.cloud(cloud.exchange("svc-a", cloud.identity(types.TypeUser, "u1"))), access: ConfiguredAccounts, identity: "user:u1", attributed: "svc-a"},
		{name: "cloud wildcard internal token", policy: allowlisted, token: cloud.cloud(cloud.service("svc-a", "*")), access: ConfiguredAccounts, attributed: "svc-a"},
		{name: "cloud relay without a user", policy: allowlisted, token: cloud.cloud(cloud.exchange("relay", cloud.service("svc-a", testNamespace))), access: Refused},
		{name: "cloud relay of a user call", policy: allowlisted, token: cloud.cloud(cloud.exchange("relay", cloud.exchange("svc-a", cloud.identity(types.TypeUser, "u1")))), access: RequestBYOK, identity: "user:u1"},
		{name: "cloud front-door user", policy: allowlisted, token: cloud.frontDoor(types.TypeUser, "u1"), access: RequestBYOK, identity: "user:u1"},
		{name: "cloud front-door service account", policy: allowlisted, token: cloud.frontDoor(types.TypeServiceAccount, "sa1"), access: RequestBYOK, identity: "service-account:sa1"},
		{name: "cloud front-door anonymous", policy: allowlisted, token: cloud.frontDoor(types.TypeAnonymous, "anon"), access: Refused},
		{name: "cloud service off the allowlist", policy: allowlisted, token: cloud.cloud(cloud.service("svc-b", testNamespace)), access: Refused},
		{name: "cloud case near-match", policy: allowlisted, token: cloud.cloud(cloud.service("SVC-A", testNamespace)), access: Refused},
		{name: "cloud whitespace near-match", policy: allowlisted, token: cloud.cloud(cloud.service("svc-a ", testNamespace)), access: Refused},
		{name: "cloud empty allowlist service", policy: emptyAllowlist, token: cloud.cloud(cloud.service("svc-a", testNamespace)), access: Refused},
		{name: "cloud empty allowlist user", policy: emptyAllowlist, token: cloud.frontDoor(types.TypeUser, "u1"), access: RequestBYOK, identity: "user:u1"},
		{name: "self-hosted service account", policy: selfHosted, keys: selfHost, token: self.exchange(platformhost.GrafanaService, self.identity(types.TypeServiceAccount, "sa1")), access: ConfiguredAccounts, identity: "service-account:sa1"},
		{name: "self-hosted user", policy: selfHosted, keys: selfHost, token: self.exchange(platformhost.GrafanaService, self.identity(types.TypeUser, "u1")), access: ConfiguredAccounts, identity: "user:u1"},
		{name: "self-hosted service with a user", policy: selfHosted, keys: selfHost, token: self.exchange(platformhost.GrafanaService, self.exchange("svc-a", self.identity(types.TypeUser, "u1"))), access: ConfiguredAccounts, identity: "user:u1"},
		{name: "self-hosted anonymous", policy: selfHosted, keys: selfHost, token: self.exchange(platformhost.GrafanaService, self.identity(types.TypeAnonymous, "anon")), access: Refused},
		{name: "self-hosted service without a user", policy: selfHosted, keys: selfHost, token: self.exchange(platformhost.GrafanaService, self.service("svc-a", testNamespace)), access: Refused},
		{name: "missing route namespace", policy: allowlisted, token: cloud.cloud(cloud.service("svc-a", testNamespace)), namespace: "-", access: Refused},
		{name: "cloud allowlisted service for another namespace", policy: allowlisted, token: cloud.cloud(cloud.service("svc-a", testNamespace)), namespace: otherNamespace, access: Refused},
		{name: "cloud front-door user for another namespace", policy: allowlisted, token: cloud.frontDoor(types.TypeUser, "u1"), namespace: otherNamespace, access: Refused},
		{name: "self-hosted user for another namespace", policy: selfHosted, keys: selfHost, token: self.exchange(platformhost.GrafanaService, self.identity(types.TypeUser, "u1")), namespace: otherNamespace, access: Refused},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keys := cloudHost
			if tc.keys != nil {
				keys = tc.keys
			}
			authenticator, err := NewPlatformAuthenticator(PlatformConfig{PluginID: testPluginID, Group: testGroup, Keys: keys.Keys()})
			require.NoError(t, err)
			info, err := authenticator.Authenticate(context.Background(), platformTokens{access: tc.token})
			require.NoError(t, err)
			namespace := tc.namespace
			switch tc.namespace {
			case "":
				namespace = testNamespace
			case "-":
				namespace = ""
			}
			caller := tc.policy.Decide(info, namespace)
			if tc.access == Refused {
				assert.Equal(t, Caller{access: Refused}, caller)
				return
			}
			assert.Equal(t, tc.access, caller.AccountAccess())
			assert.Equal(t, namespace, caller.Namespace)
			assert.Equal(t, tc.identity, caller.Identity)
			assert.Equal(t, tc.attributed, caller.AttributedService())
		})
	}
}

func TestAccessMiddleware(t *testing.T) {
	policy := AccessPolicy{Mode: AccessModeCloud, InternalServices: []string{"svc-a"}}
	extra := func(service, identity string) types.AuthInfo {
		var claims authn.Claims[authn.AccessTokenClaims]
		claims.Rest.Namespace = testNamespace
		if identity != "" {
			typ, uid, err := types.ParseTypeID(identity)
			require.NoError(t, err)
			claims.Rest.Actor = &authn.ActorClaims{IDTokenClaims: authn.IDTokenClaims{Type: typ, Identifier: uid}}
		}
		return platformAuthInfo{AuthInfo: authn.NewAccessTokenAuthInfo(claims), directCaller: service}
	}
	tests := []struct {
		name   string
		info   types.AuthInfo
		route  bool
		access AccountAccess
	}{
		{name: "configured", info: extra("svc-a", ""), route: true, access: ConfiguredAccounts},
		{name: "byok", info: extra("front-door", "user:u1"), route: true, access: RequestBYOK},
		{name: "refused", info: extra("svc-b", ""), route: true, access: Refused},
		{name: "no identity", route: true, access: Refused},
		{name: "no route info", info: extra("svc-a", ""), access: Refused},
		{name: "spoofed headers are ignored", info: extra("front-door", ""), route: true, access: Refused},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var served, refused int
			var observed []AccountAccess
			handler := AccessMiddleware(policy, func(w http.ResponseWriter) {
				refused++
				w.WriteHeader(http.StatusForbidden)
			}, func(_ context.Context, caller Caller) {
				observed = append(observed, caller.AccountAccess())
			}, http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				served++
				caller, ok := CallerFromContext(request.Context())
				require.True(t, ok)
				assert.Equal(t, testNamespace, caller.Namespace)
			}))
			request := httptest.NewRequest(http.MethodPost, "/", nil)
			request.Header.Set("X-Scope-OrgID", "stacks-2")
			request.Header.Set("X-Grafana-Id", "spoofed")
			ctx := request.Context()
			if tc.info != nil {
				ctx = types.WithAuthInfo(ctx, tc.info)
			}
			if tc.route {
				ctx = resource.WithRouteRequestInfo(ctx, &resource.RouteRequestInfo{FullIdentifier: resource.FullIdentifier{Namespace: testNamespace}})
			}
			handler.ServeHTTP(httptest.NewRecorder(), request.WithContext(ctx))
			assert.Equal(t, []AccountAccess{tc.access}, observed)
			assert.Equal(t, tc.access == Refused, refused == 1)
			assert.Equal(t, tc.access != Refused, served == 1)
		})
	}
}
