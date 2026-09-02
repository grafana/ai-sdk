package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAuthenticator_UnsafeAudiencesAndWarning(t *testing.T) {
	warnings := 0
	authenticator, err := NewAuthenticator(BuildConfig{
		Unsafe:    true,
		Audiences: []string{"custom-audience"},
		Warn: func(message string) {
			warnings++
			assert.Equal(t, unsafeAuthenticationWarning, message)
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, warnings)

	private := generateSigningKey(t)
	valid := signAccessToken(t, private, "key", []string{"custom-audience"})
	info, err := authenticator.Authenticate(context.Background(), tokenProvider{accessToken: valid})
	require.NoError(t, err)
	assert.Equal(t, "service", info.GetExtra()[authn.ServiceIdentityKey][0])

	invalid := signAccessToken(t, private, "key", []string{"other"})
	_, err = authenticator.Authenticate(context.Background(), tokenProvider{accessToken: invalid})
	require.ErrorIs(t, err, authn.ErrInvalidAudience)

	_, err = authenticator.Authenticate(context.Background(), tokenProvider{accessToken: valid, idToken: "not-a-jwt"})
	require.Error(t, err)
	_, err = authenticator.Authenticate(context.Background(), tokenProvider{
		accessToken: valid,
		idToken:     signIDToken(t, private, "key", "stack-other"),
	})
	require.Error(t, err)
}

func TestNewAuthenticator_SharesOneKeyRetriever(t *testing.T) {
	private := generateSigningKey(t)
	keys := &countingKeyRetriever{key: jose.JSONWebKey{Key: &private.PublicKey, KeyID: "shared", Algorithm: string(jose.ES256), Use: "sig"}}
	authenticator, err := NewAuthenticator(BuildConfig{Audiences: []string{"ai-sdk"}, Keys: keys})
	require.NoError(t, err)

	provider := tokenProvider{
		accessToken: signAccessToken(t, private, "shared", []string{"ai-sdk"}),
		idToken:     signIDToken(t, private, "shared", "stack-1"),
	}
	info, err := authenticator.Authenticate(context.Background(), provider)
	require.NoError(t, err)
	assert.Equal(t, "stack-1", info.GetNamespace())
	assert.Equal(t, 2, keys.CallCount())
}

func TestNewAuthenticator_RejectsInvalidConstruction(t *testing.T) {
	for _, config := range []BuildConfig{
		{},
		{Audiences: []string{""}},
		{Audiences: []string{"one", "one"}},
		{Audiences: []string{"ai-sdk"}},
	} {
		authenticator, err := NewAuthenticator(config)
		require.Error(t, err)
		assert.Nil(t, authenticator)
	}
}

type countingKeyRetriever struct {
	mu    sync.Mutex
	key   jose.JSONWebKey
	calls int
}

func (retriever *countingKeyRetriever) Get(context.Context, string) (*jose.JSONWebKey, error) {
	retriever.mu.Lock()
	defer retriever.mu.Unlock()
	retriever.calls++
	key := retriever.key
	return &key, nil
}

func (retriever *countingKeyRetriever) CallCount() int {
	retriever.mu.Lock()
	defer retriever.mu.Unlock()
	return retriever.calls
}

func generateSigningKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

func signAccessToken(t *testing.T, key *ecdsa.PrivateKey, keyID string, audience []string) string {
	t.Helper()
	return signToken(t, key, keyID, authn.TokenTypeAccess,
		authn.AccessTokenClaims{Namespace: "stack-1", ServiceIdentity: "service"},
		jwt.Claims{Subject: "access-policy:1", Audience: audience, Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	)
}

func signIDToken(t *testing.T, key *ecdsa.PrivateKey, keyID, namespace string) string {
	t.Helper()
	return signToken(t, key, keyID, authn.TokenTypeID,
		authn.IDTokenClaims{Identifier: "42", Type: types.TypeUser, Namespace: namespace},
		jwt.Claims{Subject: "user:42", Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	)
}

func signToken(t *testing.T, key *ecdsa.PrivateKey, keyID, tokenType string, claims ...any) string {
	t.Helper()
	options := (&jose.SignerOptions{}).WithType(jose.ContentType(tokenType)).WithHeader(jose.HeaderKey("kid"), keyID)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key}, options)
	require.NoError(t, err)
	builder := jwt.Signed(signer)
	for _, claim := range claims {
		builder = builder.Claims(claim)
	}
	token, err := builder.Serialize()
	require.NoError(t, err)
	return token
}
