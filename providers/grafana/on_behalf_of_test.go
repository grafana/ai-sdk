package grafana

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/authlib/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exchangedToken(t *testing.T, marker string) string {
	t.Helper()
	claims, err := json.Marshal(map[string]any{"exp": time.Now().Add(time.Hour).Unix(), "jti": marker})
	require.NoError(t, err)
	return "eyJhbGciOiJFUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(claims) + ".c2lnbmF0dXJl"
}

func TestOnBehalfOf_ExchangesSubjectToken(t *testing.T) {
	var mu sync.Mutex
	var expected string
	exchanges := map[string]int{}
	issued := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/exchange" {
			var request authn.TokenExchangeRequest
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			assert.Equal(t, "stacks-1", request.Namespace)
			assert.Equal(t, []string{"grafana-aigateway-app"}, request.Audiences)
			mu.Lock()
			exchanges[request.SubjectToken]++
			token := exchangedToken(t, request.SubjectToken)
			issued[token] = request.SubjectToken
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"token": token}})
			return
		}
		assert.Len(t, r.Header.Values("X-Access-Token"), 1)
		assert.Empty(t, r.Header.Values("X-Grafana-Id"))
		assert.Empty(t, r.Header.Values("Authorization"))
		mu.Lock()
		subject, ok := issued[r.Header.Get("X-Access-Token")]
		expected := expected
		mu.Unlock()
		assert.True(t, ok)
		assert.Equal(t, expected, subject)
		for _, values := range r.Header {
			for _, value := range values {
				assert.NotContains(t, value, "subject-token-")
			}
		}
		_, _ = io.WriteString(w, discoveryFixture)
	}))
	defer srv.Close()
	p, err := NewWithTokenExchange(TokenExchangeConfig{
		CAPToken: "cap", Namespace: "stacks-1", Audience: "grafana-aigateway-app",
		BaseURL: srv.URL + "/api", TokenExchangeURL: srv.URL + "/exchange", HTTPClient: srv.Client(),
	})
	require.NoError(t, err)

	for _, subject := range []string{"", "subject-token-user", "subject-token-delegated", "subject-token-user"} {
		ctx := context.Background()
		if subject != "" {
			ctx = WithOnBehalfOf(ctx, subject)
		}
		mu.Lock()
		expected = subject
		mu.Unlock()
		rows, err := p.ListModels(ctx)
		require.NoError(t, err)
		assert.Len(t, rows, 2)
	}
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, map[string]int{"": 1, "subject-token-user": 1, "subject-token-delegated": 1}, exchanges)
}

func TestOnBehalfOf_RefusedBeforeIO(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, context.Canceled
	})}
	exchange := func(namespace string) func() (*Provider, error) {
		return func() (*Provider, error) {
			return NewWithTokenExchange(TokenExchangeConfig{CAPToken: "cap", Namespace: namespace, Audience: "grafana-aigateway-app", BaseURL: "https://gateway.invalid", TokenExchangeURL: "https://exchange.invalid", HTTPClient: transport})
		}
	}
	for _, tc := range []struct {
		name    string
		new     func() (*Provider, error)
		context func(context.Context) context.Context
	}{
		{"access token flow", func() (*Provider, error) {
			return NewWithAccessToken(AccessTokenConfig{AccessToken: "access", BaseURL: "https://gateway.invalid", HTTPClient: transport})
		}, func(ctx context.Context) context.Context { return WithOnBehalfOf(ctx, "subject-token") }},
		{"service account flow", func() (*Provider, error) {
			return NewWithServiceAccountToken(ServiceAccountConfig{Token: "glsa_token", BaseURL: "https://gateway.invalid", HTTPClient: transport})
		}, func(ctx context.Context) context.Context { return WithOnBehalfOf(ctx, "subject-token") }},
		{"wildcard namespace", exchange("*"), func(ctx context.Context) context.Context { return WithOnBehalfOf(ctx, "subject-token") }},
		{"empty subject", exchange("stacks-1"), func(ctx context.Context) context.Context { return WithOnBehalfOf(ctx, "") }},
		{"unsafe subject", exchange("stacks-1"), func(ctx context.Context) context.Context { return WithOnBehalfOf(ctx, "subject\r\ntoken") }},
		{"acting-user conflict", exchange("stacks-1"), func(ctx context.Context) context.Context {
			return WithUserIDToken(WithOnBehalfOf(ctx, "subject-token"), "user-token")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := tc.new()
			require.NoError(t, err)
			_, err = p.ListModels(tc.context(context.Background()))
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "subject-token")
		})
	}
	assert.Zero(t, calls.Load())
}
