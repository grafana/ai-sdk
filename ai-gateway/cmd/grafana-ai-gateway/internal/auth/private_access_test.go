package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/grafana/authlib/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivateAccess_Admission(t *testing.T) {
	key := generateSigningKey(t)
	keys := &countingKeyRetriever{key: jose.JSONWebKey{Key: &key.PublicKey, KeyID: "key", Algorithm: string(jose.ES256), Use: "sig"}}
	verifier, err := NewAuthenticator(keys, []string{"ai-sdk"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name, namespace, service, audience, tokenType, idNamespace string
		expired, denied                                            bool
	}{
		{name: "concrete", namespace: "stacks-123", service: "any-service"},
		{name: "no service identity", namespace: "stacks-123"},
		{name: "wildcard", namespace: "*"},
		{name: "wildcard acting user", namespace: "*", idNamespace: "stacks-456"},
		{name: "acting user cannot remain wildcard", namespace: "*", idNamespace: "*", denied: true},
		{name: "concrete acting user", namespace: "stacks-123", idNamespace: "stacks-123"},
		{name: "namespace mismatch", namespace: "stacks-123", idNamespace: "stacks-456", denied: true},
		{name: "legacy stack", namespace: "stack-123", denied: true},
		{name: "organization", namespace: "orgs-123", denied: true},
		{name: "empty namespace", denied: true},
		{name: "zero stack", namespace: "stacks-0", denied: true},
		{name: "negative stack", namespace: "stacks--1", denied: true},
		{name: "overflow stack", namespace: "stacks-9223372036854775808", denied: true},
		{name: "audience", namespace: "stacks-123", audience: "other", denied: true},
		{name: "expired", namespace: "stacks-123", expired: true, denied: true},
		{name: "ID token as access", namespace: "stacks-123", tokenType: authn.TokenTypeID, denied: true},
	} {
		for _, header := range []string{"X-Access-Token", "Authorization"} {
			t.Run(tc.name+"/"+header, func(t *testing.T) {
				audience, typ := tc.audience, tc.tokenType
				if audience == "" {
					audience = "ai-sdk"
				}
				if typ == "" {
					typ = authn.TokenTypeAccess
				}
				expires := time.Now().Add(time.Hour)
				if tc.expired {
					expires = time.Now().Add(-time.Hour)
				}
				token := signToken(t, key, "key", typ,
					authn.AccessTokenClaims{Namespace: tc.namespace, ServiceIdentity: tc.service},
					jwt.Claims{Subject: "access-policy:1", Audience: []string{audience}, Expiry: jwt.NewNumericDate(expires)},
				)
				body := &unreadBody{}
				r := httptest.NewRequest(http.MethodPost, "/protected", body)
				if header == "Authorization" {
					token = "Bearer " + token
				}
				r.Header.Set(header, token)
				if tc.idNamespace != "" {
					r.Header.Set("X-Grafana-Id", signIDToken(t, key, "key", tc.idNamespace))
				}
				calls := 0
				h := Middleware(NewAccessTokenAuthenticator(verifier), func(w http.ResponseWriter) { w.WriteHeader(http.StatusUnauthorized) }, func(context.Context, Observation) {}, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					calls++
					caller, ok := CallerFromContext(request.Context())
					require.True(t, ok)
					wantNamespace := tc.namespace
					if tc.idNamespace != "" {
						wantNamespace = tc.idNamespace
					}
					assert.Equal(t, wantNamespace, caller.Namespace)
					assert.Equal(t, tc.service, caller.Service)
					assert.Equal(t, ConfiguredAccounts, caller.AccountAccess())
					if tc.idNamespace != "" {
						require.NotNil(t, caller.ActingUser)
						assert.Equal(t, "user:42", caller.ActingUser.Subject)
					} else {
						assert.Nil(t, caller.ActingUser)
						assert.Equal(t, "access-policy:1", caller.Subject)
					}
					w.WriteHeader(http.StatusNoContent)
				}))
				response := httptest.NewRecorder()
				h.ServeHTTP(response, r)
				if tc.denied {
					assert.Equal(t, http.StatusUnauthorized, response.Code)
					assert.Zero(t, calls)
				} else {
					assert.Equal(t, http.StatusNoContent, response.Code)
					assert.Equal(t, 1, calls)
				}
				assert.Zero(t, body.reads)
			})
		}
	}
}
