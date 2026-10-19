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

type chains struct {
	t    *testing.T
	host *platformhost.Host
}

func (c chains) exchange(service, subject string) string {
	c.t.Helper()
	token, err := c.host.Exchange(context.Background(), service, authn.TokenExchangeRequest{Namespace: testNamespace, Audiences: []string{testPluginID}, SubjectToken: subject})
	require.NoError(c.t, err)
	return token
}

func (c chains) service(name, namespace string) string {
	c.t.Helper()
	token, err := c.host.ServiceToken(name, namespace)
	require.NoError(c.t, err)
	return token
}

func (c chains) identity(identityType types.IdentityType, uid string) string {
	c.t.Helper()
	token, err := c.host.IDToken(identityType, uid, testNamespace)
	require.NoError(c.t, err)
	return token
}

func (c chains) cloud(subject string) string {
	return c.exchange(platformhost.RouterService, subject)
}

func (c chains) frontDoor(identityType types.IdentityType, uid string) string {
	return c.cloud(c.exchange(platformhost.FrontDoorService, c.identity(identityType, uid)))
}

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
		decision   Decision
		service    string
		identity   string
		attributed string
	}{
		{name: "cloud allowlisted direct service", policy: allowlisted, token: cloud.cloud(cloud.service("svc-a", testNamespace)), decision: DecisionConfigured, service: "svc-a", attributed: "svc-a"},
		{name: "cloud allowlisted service for a user", policy: allowlisted, token: cloud.cloud(cloud.exchange("svc-a", cloud.identity(types.TypeUser, "u1"))), decision: DecisionConfigured, service: "svc-a", identity: "user:u1", attributed: "svc-a"},
		{name: "cloud wildcard internal token", policy: allowlisted, token: cloud.cloud(cloud.service("svc-a", "*")), decision: DecisionConfigured, service: "svc-a", attributed: "svc-a"},
		{name: "cloud relay without a user", policy: allowlisted, token: cloud.cloud(cloud.exchange("relay", cloud.service("svc-a", testNamespace))), decision: DecisionRefused},
		{name: "cloud relay of a user call", policy: allowlisted, token: cloud.cloud(cloud.exchange("relay", cloud.exchange("svc-a", cloud.identity(types.TypeUser, "u1")))), decision: DecisionBYOK, service: "relay", identity: "user:u1"},
		{name: "cloud front-door user", policy: allowlisted, token: cloud.frontDoor(types.TypeUser, "u1"), decision: DecisionBYOK, service: platformhost.FrontDoorService, identity: "user:u1"},
		{name: "cloud front-door service account", policy: allowlisted, token: cloud.frontDoor(types.TypeServiceAccount, "sa1"), decision: DecisionBYOK, service: platformhost.FrontDoorService, identity: "service-account:sa1"},
		{name: "cloud front-door anonymous", policy: allowlisted, token: cloud.frontDoor(types.TypeAnonymous, "anon"), decision: DecisionRefused},
		{name: "cloud service off the allowlist", policy: allowlisted, token: cloud.cloud(cloud.service("svc-b", testNamespace)), decision: DecisionRefused},
		{name: "cloud case near-match", policy: allowlisted, token: cloud.cloud(cloud.service("SVC-A", testNamespace)), decision: DecisionRefused},
		{name: "cloud whitespace near-match", policy: allowlisted, token: cloud.cloud(cloud.service("svc-a ", testNamespace)), decision: DecisionRefused},
		{name: "cloud empty allowlist service", policy: emptyAllowlist, token: cloud.cloud(cloud.service("svc-a", testNamespace)), decision: DecisionRefused},
		{name: "cloud empty allowlist user", policy: emptyAllowlist, token: cloud.frontDoor(types.TypeUser, "u1"), decision: DecisionBYOK, service: platformhost.FrontDoorService, identity: "user:u1"},
		{name: "self-hosted service account", policy: selfHosted, keys: selfHost, token: self.exchange(platformhost.GrafanaService, self.identity(types.TypeServiceAccount, "sa1")), decision: DecisionConfigured, identity: "service-account:sa1"},
		{name: "self-hosted user", policy: selfHosted, keys: selfHost, token: self.exchange(platformhost.GrafanaService, self.identity(types.TypeUser, "u1")), decision: DecisionConfigured, identity: "user:u1"},
		{name: "self-hosted service with a user", policy: selfHosted, keys: selfHost, token: self.exchange(platformhost.GrafanaService, self.exchange("svc-a", self.identity(types.TypeUser, "u1"))), decision: DecisionConfigured, service: "svc-a", identity: "user:u1"},
		{name: "self-hosted anonymous", policy: selfHosted, keys: selfHost, token: self.exchange(platformhost.GrafanaService, self.identity(types.TypeAnonymous, "anon")), decision: DecisionRefused},
		{name: "self-hosted service without a user", policy: selfHosted, keys: selfHost, token: self.exchange(platformhost.GrafanaService, self.service("svc-a", testNamespace)), decision: DecisionRefused},
		{name: "missing route namespace", policy: allowlisted, token: cloud.cloud(cloud.service("svc-a", testNamespace)), namespace: "-", decision: DecisionRefused},
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
			namespace := testNamespace
			if tc.namespace == "-" {
				namespace = ""
			}
			caller, decision := tc.policy.Decide(info, namespace)
			assert.Equal(t, tc.decision, decision)
			if decision == DecisionRefused {
				assert.Equal(t, Caller{}, caller)
				return
			}
			assert.Equal(t, testNamespace, caller.Namespace)
			assert.Equal(t, tc.service, caller.Service)
			assert.Equal(t, tc.identity, caller.Identity)
			assert.Equal(t, tc.attributed, caller.AttributedService())
			wantAccess := ConfiguredAccounts
			if decision == DecisionBYOK {
				wantAccess = RequestBYOK
			}
			assert.Equal(t, wantAccess, caller.AccountAccess())
		})
	}
}

func TestAccessMiddleware(t *testing.T) {
	policy := AccessPolicy{Mode: AccessModeCloud, InternalServices: []string{"svc-a"}}
	extra := func(service, identity string) types.AuthInfo {
		values := map[string][]string{}
		if service != "" {
			values[DirectCallerServiceKey] = []string{service}
		}
		if identity != "" {
			values[IdentityActorKey] = []string{identity}
		}
		return platformAuthInfo{AuthInfo: authn.NewAccessTokenAuthInfo(authn.Claims[authn.AccessTokenClaims]{}), extra: values}
	}
	tests := []struct {
		name     string
		info     types.AuthInfo
		route    bool
		decision Decision
	}{
		{name: "configured", info: extra("svc-a", ""), route: true, decision: DecisionConfigured},
		{name: "byok", info: extra("front-door", "user:u1"), route: true, decision: DecisionBYOK},
		{name: "refused", info: extra("svc-b", ""), route: true, decision: DecisionRefused},
		{name: "no identity", route: true, decision: DecisionRefused},
		{name: "no route info", info: extra("svc-a", ""), decision: DecisionRefused},
		{name: "spoofed headers are ignored", info: extra("front-door", ""), route: true, decision: DecisionRefused},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var served, refused int
			var observed []Decision
			handler := AccessMiddleware(policy, func(w http.ResponseWriter) {
				refused++
				w.WriteHeader(http.StatusForbidden)
			}, func(_ context.Context, decision Decision, caller *Caller) {
				observed = append(observed, decision)
				assert.Equal(t, decision == DecisionRefused, caller == nil)
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
			assert.Equal(t, []Decision{tc.decision}, observed)
			assert.Equal(t, tc.decision == DecisionRefused, refused == 1)
			assert.Equal(t, tc.decision != DecisionRefused, served == 1)
		})
	}
}
