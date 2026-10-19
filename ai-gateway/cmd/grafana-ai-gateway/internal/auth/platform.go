package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"

	"github.com/go-jose/go-jose/v4"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
)

const (
	// EnvSigningKeysURL names the URL of the platform's JSON Web Key Set.
	EnvSigningKeysURL = "GRAFANA_JWKS_URL"
	// EnvSigningKeys names an inline JSON Web Key Set of public signing keys.
	EnvSigningKeys = "GRAFANA_JWKS"
)

const (
	// DirectCallerServiceKey is the AuthInfo extra holding the signed service
	// identity of the first actor, the service that called the plugin host.
	DirectCallerServiceKey = "aigateway.directCallerService"
)

// ErrNoSigningKeys reports that neither signing key source is configured.
var ErrNoSigningKeys = errors.New("gateway auth: no platform signing keys configured")

// SigningKeys returns the platform signing keys from exactly one of
// GRAFANA_JWKS_URL or GRAFANA_JWKS, following the plugin host's rules.
func SigningKeys(lookupEnv func(string) (string, bool)) (authn.KeyRetriever, error) {
	keysURL, _ := lookupEnv(EnvSigningKeysURL)
	keysJSON, _ := lookupEnv(EnvSigningKeys)
	switch {
	case keysURL == "" && keysJSON == "":
		return nil, ErrNoSigningKeys
	case keysURL != "" && keysJSON != "":
		return nil, fmt.Errorf("gateway auth: set only one of %s and %s", EnvSigningKeysURL, EnvSigningKeys)
	case keysURL != "":
		parsed, err := url.Parse(keysURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, fmt.Errorf("gateway auth: %s must be an absolute http or https URL", EnvSigningKeysURL)
		}
		return authn.NewKeyRetriever(authn.KeyRetrieverConfig{SigningKeysURL: keysURL}), nil
	default:
		return parseSigningKeys(keysJSON)
	}
}

type staticKeys map[string]*jose.JSONWebKey

func (keys staticKeys) Get(_ context.Context, keyID string) (*jose.JSONWebKey, error) {
	if key, ok := keys[keyID]; ok {
		return key, nil
	}
	return nil, authn.ErrInvalidSigningKey
}

func parseSigningKeys(raw string) (staticKeys, error) {
	var set jose.JSONWebKeySet
	if err := json.Unmarshal([]byte(raw), &set); err != nil {
		return nil, fmt.Errorf("gateway auth: %s is not a JSON Web Key Set", EnvSigningKeys)
	}
	if len(set.Keys) == 0 {
		return nil, fmt.Errorf("gateway auth: %s has no keys", EnvSigningKeys)
	}
	keys := make(staticKeys, len(set.Keys))
	for i := range set.Keys {
		key := &set.Keys[i]
		switch {
		case key.KeyID == "":
			return nil, fmt.Errorf("gateway auth: %s key %d has no key ID", EnvSigningKeys, i)
		case !key.IsPublic():
			return nil, fmt.Errorf("gateway auth: %s key %d is not a public key", EnvSigningKeys, i)
		case !key.Valid():
			return nil, fmt.Errorf("gateway auth: %s key %d is invalid", EnvSigningKeys, i)
		case key.Use != "" && key.Use != "sig":
			return nil, fmt.Errorf("gateway auth: %s key %d is not a signing key", EnvSigningKeys, i)
		}
		if _, duplicate := keys[key.KeyID]; duplicate {
			return nil, fmt.Errorf("gateway auth: %s has duplicate key IDs", EnvSigningKeys)
		}
		keys[key.KeyID] = key
	}
	return keys, nil
}

// PlatformConfig configures verification of delegated access tokens.
type PlatformConfig struct {
	// PluginID and Group are the accepted token audiences.
	PluginID string
	Group    string
	// Keys verifies token signatures. Without keys, Unsafe must be set.
	Keys authn.KeyRetriever
	// Unsafe parses tokens without verifying signatures, for local development.
	Unsafe bool
}

// NewPlatformAuthenticator returns the plugin authenticator for delegated
// access tokens. Its AuthInfo extras add the direct caller's service
// identity from the verified actor chain.
func NewPlatformAuthenticator(config PlatformConfig) (authn.Authenticator, error) {
	if config.PluginID == "" || config.Group == "" {
		return nil, errors.New("gateway auth: plugin ID and API group audiences are required")
	}
	verifierConfig := authn.VerifierConfig{AllowedAudiences: []string{config.PluginID, config.Group}}
	switch {
	case config.Unsafe && config.Keys == nil:
		return platformAuthenticator{verifier: authn.NewUnsafeAccessTokenVerifier(verifierConfig)}, nil
	case !config.Unsafe && config.Keys != nil:
		return platformAuthenticator{verifier: authn.NewAccessTokenVerifier(verifierConfig, config.Keys)}, nil
	default:
		return nil, errors.New("gateway auth: configure either signing keys or unsafe verification")
	}
}

type platformAuthenticator struct {
	verifier *authn.AccessTokenVerifier
}

func (a platformAuthenticator) Authenticate(ctx context.Context, provider authn.TokenProvider) (types.AuthInfo, error) {
	token, ok := provider.AccessToken(ctx)
	if !ok {
		return nil, authn.ErrMissingRequiredToken
	}
	claims, err := a.verifier.Verify(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("gateway auth: verifying access token: %w", err)
	}
	extra := map[string][]string{}
	if actor := claims.Rest.Actor; actor != nil && actor.ServiceIdentity != "" {
		extra[DirectCallerServiceKey] = []string{actor.ServiceIdentity}
	}
	return platformAuthInfo{AuthInfo: authn.NewAccessTokenAuthInfo(*claims), extra: extra}, nil
}

type platformAuthInfo struct {
	types.AuthInfo
	extra map[string][]string
}

func (info platformAuthInfo) GetExtra() map[string][]string {
	result := info.AuthInfo.GetExtra()
	for key, values := range info.extra {
		result[key] = slices.Clone(values)
	}
	return result
}
