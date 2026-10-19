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

func delegate(t *testing.T, host *platformhost.Host, service, subject string) string {
	t.Helper()
	token, err := host.Exchange(context.Background(), service, authn.TokenExchangeRequest{Namespace: testNamespace, Audiences: []string{testPluginID}, SubjectToken: subject})
	require.NoError(t, err)
	return token
}

func TestPlatformAuthenticator_ActorChains(t *testing.T) {
	host := newHost(t, platformhost.TopologyCloud)
	service := func(name string) string {
		token, err := host.ServiceToken(name, testNamespace)
		require.NoError(t, err)
		return token
	}
	identity := func(identityType types.IdentityType, uid string) string {
		token, err := host.IDToken(identityType, uid, testNamespace)
		require.NoError(t, err)
		return token
	}
	viaFrontDoor := func(identityType types.IdentityType, uid string) string {
		return delegate(t, host, platformhost.FrontDoorService, identity(identityType, uid))
	}
	tests := []struct {
		name    string
		token   string
		service string
		actor   string
	}{
		{name: "direct service", token: delegate(t, host, platformhost.RouterService, service("svc-a")), service: "svc-a"},
		{name: "on behalf of a user", token: delegate(t, host, platformhost.RouterService, delegate(t, host, "svc-a", identity(types.TypeUser, "u1"))), service: "svc-a", actor: "user:u1"},
		{name: "plugin on behalf of a delegated user", token: delegate(t, host, platformhost.RouterService, delegate(t, host, "other-app-operator", delegate(t, host, platformhost.RouterService, viaFrontDoor(types.TypeUser, "u1")))), service: "other-app-operator", actor: "user:u1"},
		{name: "relayed", token: delegate(t, host, platformhost.RouterService, delegate(t, host, "relay", service("svc-a"))), service: "relay"},
		{name: "front-door user", token: delegate(t, host, platformhost.RouterService, viaFrontDoor(types.TypeUser, "u1")), service: platformhost.FrontDoorService, actor: "user:u1"},
		{name: "front-door service account", token: delegate(t, host, platformhost.RouterService, viaFrontDoor(types.TypeServiceAccount, "sa1")), service: platformhost.FrontDoorService, actor: "service-account:sa1"},
		{name: "front-door anonymous", token: delegate(t, host, platformhost.RouterService, viaFrontDoor(types.TypeAnonymous, "anon")), service: platformhost.FrontDoorService},
		{name: "no actor", token: service("svc-a")},
	}
	authenticator, err := NewPlatformAuthenticator(PlatformConfig{PluginID: testPluginID, Group: testGroup, Keys: host.Keys()})
	require.NoError(t, err)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spoofed := identity(types.TypeUser, "spoofed")
			info, err := authenticator.Authenticate(context.Background(), platformTokens{access: tc.token, id: spoofed})
			require.NoError(t, err)
			extra := info.GetExtra()
			assert.Equal(t, optional(tc.service), extra[DirectCallerServiceKey])
			assert.Equal(t, optional(tc.actor), extra[IdentityActorKey])
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
	info, err := authenticator.Authenticate(context.Background(), platformTokens{access: delegate(t, host, platformhost.GrafanaService, account)})
	require.NoError(t, err)
	assert.Empty(t, info.GetExtra()[DirectCallerServiceKey])
	assert.Equal(t, []string{"service-account:sa1"}, info.GetExtra()[IdentityActorKey])
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
		"foreign signer":   {access: delegate(t, other, platformhost.RouterService, foreignSubject)},
		"foreign audience": {access: foreignAudience},
		"ID token":         {access: idToken},
		"malformed":        {access: "not-a-token"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := authenticator.Authenticate(context.Background(), provider)
			require.Error(t, err)
		})
	}

	unsafe, err := NewPlatformAuthenticator(PlatformConfig{PluginID: testPluginID, Group: testGroup, Unsafe: true})
	require.NoError(t, err)
	info, err := unsafe.Authenticate(context.Background(), platformTokens{access: delegate(t, other, platformhost.RouterService, foreignSubject)})
	require.NoError(t, err)
	assert.Equal(t, []string{"svc-a"}, info.GetExtra()[DirectCallerServiceKey])
	_, err = unsafe.Authenticate(context.Background(), platformTokens{access: foreignAudience})
	require.Error(t, err)

	for name, config := range map[string]PlatformConfig{
		"no audiences":    {Keys: host.Keys()},
		"neither source":  {PluginID: testPluginID, Group: testGroup},
		"keys and unsafe": {PluginID: testPluginID, Group: testGroup, Keys: host.Keys(), Unsafe: true},
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
		{name: "inline keys", env: map[string]string{EnvSigningKeys: string(public)}, valid: true},
		{name: "keys URL", env: map[string]string{EnvSigningKeysURL: "http://auth.internal/jwks"}, valid: true},
		{name: "neither"},
		{name: "both", env: map[string]string{EnvSigningKeys: string(public), EnvSigningKeysURL: "https://auth.test/jwks"}},
		{name: "relative URL", env: map[string]string{EnvSigningKeysURL: "/jwks"}},
		{name: "non-http URL", env: map[string]string{EnvSigningKeysURL: "file:///jwks"}},
		{name: "not JSON", env: map[string]string{EnvSigningKeys: "keys"}},
		{name: "empty set", env: map[string]string{EnvSigningKeys: keySet()}},
		{name: "private key", env: map[string]string{EnvSigningKeys: keySet(jose.JSONWebKey{Key: private, KeyID: "private", Algorithm: string(jose.ES256)})}},
		{name: "missing key ID", env: map[string]string{EnvSigningKeys: keySet(jose.JSONWebKey{Key: &private.PublicKey, Algorithm: string(jose.ES256)})}},
		{name: "encryption key", env: map[string]string{EnvSigningKeys: keySet(jose.JSONWebKey{Key: &private.PublicKey, KeyID: "enc", Algorithm: string(jose.ES256), Use: "enc"})}},
		{name: "duplicate key IDs", env: map[string]string{EnvSigningKeys: keySet(jose.JSONWebKey{Key: &private.PublicKey, KeyID: "a", Algorithm: string(jose.ES256)}, jose.JSONWebKey{Key: &private.PublicKey, KeyID: "a", Algorithm: string(jose.ES256)})}},
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
