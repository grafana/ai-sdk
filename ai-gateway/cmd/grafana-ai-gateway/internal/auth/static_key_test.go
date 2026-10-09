package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticKeyAuthenticator_Admission(t *testing.T) {
	key := "opaque._~+/-key=="
	identities := []StaticIdentity{{Name: "deployment", KeyDigest: sha256.Sum256([]byte(key))}, {Name: "second", KeyDigest: sha256.Sum256([]byte("eyJhbGci.eyJleHA.signature"))}}
	authenticator, err := NewStaticKeyAuthenticator(identities)
	require.NoError(t, err)
	identities[0] = StaticIdentity{Name: "mutated", KeyDigest: sha256.Sum256([]byte("other"))}
	for _, tc := range []struct {
		name     string
		headers  http.Header
		identity string
	}{
		{"bearer", http.Header{"Authorization": {"Bearer " + key}}, "deployment"},
		{"case scheme", http.Header{"authorization": {"bEaReR " + key}}, "deployment"},
		{"access", http.Header{"X-Access-Token": {key}}, "deployment"},
		{"opaque JWT", http.Header{"X-Access-Token": {"eyJhbGci.eyJleHA.signature"}}, "second"},
		{"missing", http.Header{}, ""}, {"wrong", http.Header{"X-Access-Token": {"wrong"}}, ""},
		{"both", http.Header{"X-Access-Token": {key}, "Authorization": {"Bearer " + key}}, ""},
		{"duplicate", http.Header{"Authorization": {"Bearer " + key, "Bearer " + key}}, ""},
		{"casing", http.Header{"Authorization": {"Bearer " + key}, "authorization": {"Bearer " + key}}, ""},
		{"nil casing", http.Header{"Authorization": {"Bearer " + key}, "authorization": nil}, ""},
		{"access casing", http.Header{"X-Access-Token": {key}, "x-access-token": {key}}, ""},
		{"joined", http.Header{"Authorization": {"Bearer " + key + ", Bearer " + key}}, ""},
		{"empty alternative", http.Header{"Authorization": {"Bearer " + key}, "X-Access-Token": {""}}, ""},
		{"unsupported", http.Header{"Authorization": {"Basic " + key}}, ""},
		{"double space", http.Header{"Authorization": {"Bearer  " + key}}, ""},
		{"prefixed access", http.Header{"X-Access-Token": {"Bearer " + key}}, ""},
		{"oversized", http.Header{"X-Access-Token": {strings.Repeat("a", 4097)}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller, err := authenticator.Authenticate(context.Background(), tc.headers)
			if tc.identity == "" {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), key)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, Caller{Source: SourceStaticKey, Service: tc.identity, Subject: "local:" + tc.identity, access: ConfiguredAccounts}, caller)
		})
	}
	for _, name := range []string{"X-Grafana-Id", "x-scope-orgid", "X-Cloud-Org-ID", "X-Access-Policy-ID"} {
		for _, values := range [][]string{nil, {""}, {"assertion"}} {
			headers := http.Header{"X-Access-Token": {key}, name: values}
			_, err := authenticator.Authenticate(context.Background(), headers)
			require.Error(t, err)
		}
	}
	var workers sync.WaitGroup
	for range 32 {
		workers.Go(func() {
			for range 100 {
				caller, err := authenticator.Authenticate(context.Background(), http.Header{"X-Access-Token": {key}})
				assert.NoError(t, err)
				assert.Equal(t, "deployment", caller.Service)
			}
		})
	}
	workers.Wait()
}

func TestStaticKeyAuthenticator_ConstructionAndSyntax(t *testing.T) {
	for _, value := range []string{"a", "a=", "a===", "a._~+/-Z09", strings.Repeat("a", 4096), "eyJ.eyJ.sig"} {
		assert.True(t, ValidStaticKey(value), value[:1])
	}
	for _, value := range []string{"", "=", "=a", "a=b", "a b", "a\t", "a\n", "a,b", "é", "a:", strings.Repeat("a", 4097)} {
		assert.False(t, ValidStaticKey(value))
	}
	identity := StaticIdentity{Name: "one", KeyDigest: sha256.Sum256([]byte("key"))}
	for _, identities := range [][]StaticIdentity{nil, make([]StaticIdentity, 65), {identity, identity}, {{Name: "bad!"}}, {identity, {Name: "two", KeyDigest: identity.KeyDigest}}} {
		_, err := NewStaticKeyAuthenticator(identities)
		require.Error(t, err)
	}
}

var _ RequestAuthenticator = staticKeyAuthenticator{}

func TestStaticKeyAuthenticator_AllIdentityPositions(t *testing.T) {
	identities := make([]StaticIdentity, 64)
	for i := range identities {
		identities[i] = StaticIdentity{Name: fmt.Sprintf("identity-%d", i), KeyDigest: sha256.Sum256([]byte(fmt.Sprintf("key-%d", i)))}
	}
	authenticator, err := NewStaticKeyAuthenticator(identities)
	require.NoError(t, err)
	for i, identity := range identities {
		caller, err := authenticator.Authenticate(context.Background(), http.Header{"X-Access-Token": {fmt.Sprintf("key-%d", i)}})
		require.NoError(t, err)
		assert.Equal(t, identity.Name, caller.Service)
	}
}

func TestStaticKeyDigest_TemporaryBuffer(t *testing.T) {
	for _, key := range []string{"short", "opaque.a_b~c+d/e-==", strings.Repeat("a", 4096)} {
		t.Run(fmt.Sprint(len(key)), func(t *testing.T) {
			buffer := []byte(key)
			want := sha256.Sum256(buffer)
			assert.Equal(t, want, digestAndClear(buffer))
			assert.Equal(t, make([]byte, len(key)), buffer)
			digest, valid := StaticKeyDigest(key)
			require.True(t, valid)
			assert.Equal(t, want, digest)
			assert.Equal(t, want, sha256.Sum256([]byte(key)))
		})
	}
	for _, key := range []string{"", "invalid space", strings.Repeat("a", 4097)} {
		digest, valid := StaticKeyDigest(key)
		assert.False(t, valid)
		assert.Equal(t, [sha256.Size]byte{}, digest)
	}
}
