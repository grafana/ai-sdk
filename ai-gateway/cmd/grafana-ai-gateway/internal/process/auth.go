package process

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sort"
	"time"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/outbound"
	"github.com/grafana/authlib/authn"
)

func buildAuthenticator(ctx context.Context, settings config.Settings, effective config.EffectiveAuth, lookupEnv config.LookupEnv, logger *slog.Logger) (gatewayauth.RequestAuthenticator, gatewayauth.Source, error) {
	var verifier authn.Authenticator
	var err error
	switch effective.Type {
	case config.AuthStaticKey:
		names := make([]string, 0, len(effective.Identities))
		for name := range effective.Identities {
			names = append(names, name)
		}
		sort.Strings(names)
		digests := make(map[string][sha256.Size]byte)
		identities := make([]gatewayauth.StaticIdentity, 0, len(names))
		for _, name := range names {
			reference := effective.Identities[name].KeyEnv
			digest, exists := digests[reference]
			if !exists {
				if lookupEnv == nil {
					return nil, "", fmt.Errorf("gateway process: static credential is unavailable")
				}
				key, ok := lookupEnv(reference)
				if !ok {
					return nil, "", fmt.Errorf("gateway process: static credential is unavailable or invalid")
				}
				var valid bool
				digest, valid = gatewayauth.StaticKeyDigest(key)
				if !valid {
					return nil, "", fmt.Errorf("gateway process: static credential is unavailable or invalid")
				}
				digests[reference] = digest
			}
			identities = append(identities, gatewayauth.StaticIdentity{Name: name, KeyDigest: digest})
		}
		authenticator, err := gatewayauth.NewStaticKeyAuthenticator(identities)
		return authenticator, gatewayauth.SourceStaticKey, err

	case config.AuthUnsafe:
		verifier, err = gatewayauth.NewUnsafeAuthenticator(settings.Audiences, func(message string) { logger.Warn(message) })
	case config.AuthJWT:
		client, clientErr := outbound.NewJWKSClient(settings.JWKSRequestTimeout, settings.JWKSResponseBytes)
		if clientErr != nil {
			return nil, "", clientErr
		}
		keys, keyErr := gatewayauth.NewJWKS(ctx, client, time.Now, gatewayauth.JWKSConfig{
			URL: effective.JWKSURL, RequestTimeout: settings.JWKSRequestTimeout, MaxKeys: settings.JWKSMaxKeys,
			RefreshInterval: settings.JWKSRefreshInterval, MaxAge: settings.JWKSMaxAge,
		})
		if keyErr != nil {
			return nil, "", keyErr
		}
		verifier, err = gatewayauth.NewAuthenticator(keys, settings.Audiences)
	default:
		return nil, "", fmt.Errorf("gateway process: authentication type is not implemented")
	}
	if err != nil {
		return nil, "", err
	}
	return gatewayauth.NewAccessTokenAuthenticator(verifier), gatewayauth.SourceAccessToken, nil
}
