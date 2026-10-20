package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/grafana/grafana-app-sdk/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const (
	testPluginID  = "grafana-aigateway-app"
	testGroup     = "aigateway.ext.grafana.app"
	testNamespace = "stacks-1"
)

type platformTokens struct{ access, id string }

func (p platformTokens) AccessToken(context.Context) (string, bool) { return p.access, p.access != "" }
func (p platformTokens) IDToken(context.Context) (string, bool)     { return p.id, p.id != "" }

func newHost(t *testing.T, topology platformhost.Topology) *platformhost.Host {
	t.Helper()
	host, err := platformhost.New(platformhost.Config{Topology: topology, PluginID: testPluginID, Group: testGroup, Version: "v1alpha1"})
	require.NoError(t, err)
	return host
}

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

func TestPlatformAuthenticator_ActorChains(t *testing.T) {
	host := newHost(t, platformhost.TopologyCloud)
	c := chains{t: t, host: host}
	tests := []struct {
		name    string
		token   string
		service string
		actor   string
	}{
		{name: "direct service", token: c.cloud(c.service("svc-a", testNamespace)), service: "svc-a"},
		{name: "on behalf of a user", token: c.cloud(c.exchange("svc-a", c.identity(types.TypeUser, "u1"))), service: "svc-a", actor: "user:u1"},
		{name: "plugin on behalf of a delegated user", token: c.cloud(c.exchange("other-app-operator", c.frontDoor(types.TypeUser, "u1"))), service: "other-app-operator", actor: "user:u1"},
		{name: "relayed", token: c.cloud(c.exchange("relay", c.service("svc-a", testNamespace))), service: "relay"},
		{name: "front-door user", token: c.frontDoor(types.TypeUser, "u1"), service: platformhost.FrontDoorService, actor: "user:u1"},
		{name: "front-door service account", token: c.frontDoor(types.TypeServiceAccount, "sa1"), service: platformhost.FrontDoorService, actor: "service-account:sa1"},
		{name: "front-door anonymous", token: c.frontDoor(types.TypeAnonymous, "anon"), service: platformhost.FrontDoorService},
		{name: "no actor", token: c.service("svc-a", testNamespace)},
	}
	authenticator, err := NewPlatformAuthenticator(PlatformConfig{PluginID: testPluginID, Group: testGroup, Keys: host.Keys()})
	require.NoError(t, err)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spoofed := c.identity(types.TypeUser, "spoofed")
			info, err := authenticator.Authenticate(context.Background(), platformTokens{access: tc.token, id: spoofed})
			require.NoError(t, err)
			extra := info.GetExtra()
			assert.Equal(t, optional(tc.service), extra[DirectCallerServiceKey])
			assert.Equal(t, tc.actor, identityActor(info))
			assert.NotContains(t, extra, "id-token")
		})
	}
}

func TestPlatformAuthenticator_SelfHostedServiceAccount(t *testing.T) {
	host := newHost(t, platformhost.TopologySelfHosted)
	account, err := host.IDToken(types.TypeServiceAccount, "sa1", testNamespace)
	require.NoError(t, err)
	authenticator, err := NewPlatformAuthenticator(PlatformConfig{PluginID: testPluginID, Group: testGroup, Keys: host.Keys()})
	require.NoError(t, err)
	info, err := authenticator.Authenticate(context.Background(), platformTokens{access: chains{t: t, host: host}.exchange(platformhost.GrafanaService, account)})
	require.NoError(t, err)
	assert.Empty(t, info.GetExtra()[DirectCallerServiceKey])
	assert.Equal(t, "service-account:sa1", identityActor(info))
}

func TestPlatformAuthenticator_Refusals(t *testing.T) {
	host := newHost(t, platformhost.TopologyCloud)
	other := newHost(t, platformhost.TopologyCloud)
	subject, err := host.ServiceToken("svc-a", testNamespace)
	require.NoError(t, err)
	foreignSubject, err := other.ServiceToken("svc-a", testNamespace)
	require.NoError(t, err)
	foreignAudience, err := host.Exchange(context.Background(), platformhost.RouterService, authn.TokenExchangeRequest{Namespace: testNamespace, Audiences: []string{"other-app"}, SubjectToken: subject})
	require.NoError(t, err)
	idToken, err := host.IDToken(types.TypeUser, "u1", testNamespace)
	require.NoError(t, err)
	authenticator, err := NewPlatformAuthenticator(PlatformConfig{PluginID: testPluginID, Group: testGroup, Keys: host.Keys()})
	require.NoError(t, err)
	for name, provider := range map[string]platformTokens{
		"missing token":    {},
		"foreign signer":   {access: chains{t: t, host: other}.cloud(foreignSubject)},
		"foreign audience": {access: foreignAudience},
		"ID token":         {access: idToken},
		"malformed":        {access: "not-a-token"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := authenticator.Authenticate(context.Background(), provider)
			require.Error(t, err)
		})
	}

	for name, config := range map[string]PlatformConfig{
		"no audiences": {Keys: host.Keys()},
		"no keys":      {PluginID: testPluginID, Group: testGroup},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewPlatformAuthenticator(config)
			require.Error(t, err)
		})
	}
}

func TestSigningKeys(t *testing.T) {
	host := newHost(t, platformhost.TopologyCloud)
	public, err := host.JWKS()
	require.NoError(t, err)
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keySet := func(keys ...jose.JSONWebKey) string {
		raw, err := json.Marshal(jose.JSONWebKeySet{Keys: keys})
		require.NoError(t, err)
		return string(raw)
	}
	tests := []struct {
		name  string
		env   map[string]string
		valid bool
	}{
		{name: "inline keys", env: map[string]string{plugin.EnvVarGrafanaJWKS: string(public)}, valid: true},
		{name: "keys URL", env: map[string]string{plugin.EnvVarGrafanaJWKSURL: "http://auth.internal/jwks"}, valid: true},
		{name: "neither"},
		{name: "both", env: map[string]string{plugin.EnvVarGrafanaJWKS: string(public), plugin.EnvVarGrafanaJWKSURL: "https://auth.test/jwks"}},
		{name: "relative URL", env: map[string]string{plugin.EnvVarGrafanaJWKSURL: "/jwks"}},
		{name: "non-http URL", env: map[string]string{plugin.EnvVarGrafanaJWKSURL: "file:///jwks"}},
		{name: "not JSON", env: map[string]string{plugin.EnvVarGrafanaJWKS: "keys"}},
		{name: "empty set", env: map[string]string{plugin.EnvVarGrafanaJWKS: keySet()}},
		{name: "private key", env: map[string]string{plugin.EnvVarGrafanaJWKS: keySet(jose.JSONWebKey{Key: private, KeyID: "private", Algorithm: string(jose.ES256)})}},
		{name: "missing key ID", env: map[string]string{plugin.EnvVarGrafanaJWKS: keySet(jose.JSONWebKey{Key: &private.PublicKey, Algorithm: string(jose.ES256)})}},
		{name: "encryption key", env: map[string]string{plugin.EnvVarGrafanaJWKS: keySet(jose.JSONWebKey{Key: &private.PublicKey, KeyID: "enc", Algorithm: string(jose.ES256), Use: "enc"})}},
		{name: "duplicate key IDs", env: map[string]string{plugin.EnvVarGrafanaJWKS: keySet(jose.JSONWebKey{Key: &private.PublicKey, KeyID: "a", Algorithm: string(jose.ES256)}, jose.JSONWebKey{Key: &private.PublicKey, KeyID: "a", Algorithm: string(jose.ES256)})}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keys, err := SigningKeys(func(name string) (string, bool) {
				value, ok := tc.env[name]
				return value, ok
			})
			if tc.valid {
				require.NoError(t, err)
				assert.NotNil(t, keys)
				return
			}
			require.Error(t, err)
		})
	}
}

func optional(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}
