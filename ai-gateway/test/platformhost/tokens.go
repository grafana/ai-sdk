// Package platformhost imitates the Grafana App Platform plugin host for
// deterministic Gateway tests. It signs delegated tokens with an in-memory key,
// builds platform-shaped actor chains and forwards HTTP requests through the
// plugin protocol v3 route service. It is test-only and is never packaged.
package platformhost

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
)

const (
	signingKeyID  = "platformhost"
	tokenLifetime = 10 * time.Minute
)

type signer struct {
	key    *ecdsa.PrivateKey
	public jose.JSONWebKey
	access *authn.AccessTokenVerifier
	id     *authn.IDTokenVerifier
}

func newSigner() (*signer, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("platformhost: generating signing key: %w", err)
	}
	s := &signer{
		key:    key,
		public: jose.JSONWebKey{Key: &key.PublicKey, KeyID: signingKeyID, Algorithm: string(jose.ES256), Use: "sig"},
	}
	s.access = authn.NewAccessTokenVerifier(authn.VerifierConfig{}, s)
	s.id = authn.NewIDTokenVerifier(authn.VerifierConfig{}, s)
	return s, nil
}

func (s *signer) Get(_ context.Context, keyID string) (*jose.JSONWebKey, error) {
	if keyID != signingKeyID {
		return nil, authn.ErrInvalidSigningKey
	}
	return &s.public, nil
}

func (s *signer) jwks() ([]byte, error) {
	return json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{s.public}})
}

func (s *signer) sign(tokenType authn.TokenType, subject string, audience []string, rest any) (string, error) {
	options := (&jose.SignerOptions{}).WithType(jose.ContentType(tokenType))
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: jose.JSONWebKey{Key: s.key, KeyID: signingKeyID}}, options)
	if err != nil {
		return "", fmt.Errorf("platformhost: creating signer: %w", err)
	}
	now := time.Now()
	standard := jwt.Claims{
		Issuer:    "platformhost",
		Subject:   subject,
		Audience:  audience,
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
		Expiry:    jwt.NewNumericDate(now.Add(tokenLifetime)),
	}
	token, err := jwt.Signed(sig).Claims(standard).Claims(rest).Serialize()
	if err != nil {
		return "", fmt.Errorf("platformhost: signing token: %w", err)
	}
	return token, nil
}

func (s *signer) accessToken(subject string, audience []string, claims authn.AccessTokenClaims) (string, error) {
	return s.sign(authn.TokenTypeAccess, subject, audience, claims)
}

func (s *signer) idToken(identityType types.IdentityType, uid, namespace string) (string, error) {
	return s.sign(authn.TokenTypeID, typedUID(identityType, uid), nil, authn.IDTokenClaims{
		Identifier: uid,
		Type:       identityType,
		Namespace:  namespace,
	})
}

func (s *signer) actorFor(ctx context.Context, token string) (*authn.ActorClaims, string, error) {
	parsed, err := authn.Parse(token)
	if err != nil {
		return nil, "", err
	}
	tokenType, err := authn.GetType(parsed)
	if err != nil {
		return nil, "", err
	}
	switch tokenType {
	case authn.TokenTypeAccess:
		claims, err := s.access.Verify(ctx, token)
		if err != nil {
			return nil, "", err
		}
		return &authn.ActorClaims{
			Subject:         claims.Subject,
			ServiceIdentity: claims.Rest.ServiceIdentity,
			Actor:           claims.Rest.Actor,
		}, claims.Rest.Namespace, nil
	case authn.TokenTypeID:
		claims, err := s.id.Verify(ctx, token)
		if err != nil {
			return nil, "", err
		}
		identity := claims.Rest
		namespace := identity.Namespace
		identity.Namespace = ""
		return &authn.ActorClaims{Subject: claims.Subject, IDTokenClaims: identity}, namespace, nil
	default:
		return nil, "", errors.New("platformhost: unsupported subject token type")
	}
}

func (s *signer) exchange(ctx context.Context, service string, request authn.TokenExchangeRequest) (string, error) {
	actor, namespace, err := s.actorFor(ctx, request.SubjectToken)
	if err != nil {
		return "", fmt.Errorf("platformhost: invalid subject token: %w", err)
	}
	if !types.NamespaceMatches(namespace, request.Namespace) {
		return "", errors.New("platformhost: subject token does not cover the requested namespace")
	}
	return s.accessToken(typedUID(types.TypeAccessPolicy, service), request.Audiences, authn.AccessTokenClaims{
		Namespace:       request.Namespace,
		ServiceIdentity: service,
		Actor:           actor,
	})
}

func typedUID(identityType types.IdentityType, uid string) string {
	return string(identityType) + ":" + uid
}
