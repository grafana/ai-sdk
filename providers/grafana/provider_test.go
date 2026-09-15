package grafana

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/registry"
	"github.com/grafana/authlib/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ registry.Provider      = (*Provider)(nil)
	_ provider.LanguageModel = (*model)(nil)
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testProvider(t *testing.T, handler http.HandlerFunc, limits *Limits) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "access-token", BaseURL: srv.URL + "/api/v1/aisdk/", Limits: limits})
	require.NoError(t, err)
	return p
}

const discoveryFixture = `{"models":[{"id":"assistant","name":"Assistant","description":"public description","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"assistant"}},{"id":"grafana/assistant","name":"Grafana Assistant","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"grafana/assistant"}}]}`

func TestNewWithAccessToken_Validation(t *testing.T) {
	valid := AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test/api/v1/aisdk"}
	for _, tc := range []struct {
		name   string
		change func(*AccessTokenConfig)
	}{
		{"empty token", func(c *AccessTokenConfig) { c.AccessToken = "" }},
		{"whitespace token", func(c *AccessTokenConfig) { c.AccessToken = " \t" }},
		{"embedded space", func(c *AccessTokenConfig) { c.AccessToken = "token value" }},
		{"header injection", func(c *AccessTokenConfig) { c.AccessToken = "token\r\nx-secret: injected" }},
		{"invalid UTF8 token", func(c *AccessTokenConfig) { c.AccessToken = string([]byte{255}) }},
		{"empty URL", func(c *AccessTokenConfig) { c.BaseURL = "" }},
		{"relative URL", func(c *AccessTokenConfig) { c.BaseURL = "/api" }},
		{"bad scheme", func(c *AccessTokenConfig) { c.BaseURL = "file:///api" }},
		{"userinfo", func(c *AccessTokenConfig) { c.BaseURL = "https://secret:password@example.test" }},
		{"query", func(c *AccessTokenConfig) { c.BaseURL += "?secret=value" }},
		{"empty query", func(c *AccessTokenConfig) { c.BaseURL += "?" }},
		{"fragment", func(c *AccessTokenConfig) { c.BaseURL += "#fragment" }},
		{"empty fragment", func(c *AccessTokenConfig) { c.BaseURL += "#" }},
		{"leading whitespace", func(c *AccessTokenConfig) { c.BaseURL = " " + c.BaseURL }},
		{"header name", func(c *AccessTokenConfig) { c.Headers = http.Header{"Bad Name": {"value"}} }},
		{"header value", func(c *AccessTokenConfig) { c.Headers = http.Header{"X-Test": {"secret\nvalue"}} }},
		{"header casing duplicate", func(c *AccessTokenConfig) { c.Headers = http.Header{"X-Test": {"a"}, "x-test": {"b"}} }},
		{"zero limits", func(c *AccessTokenConfig) { c.Limits = &Limits{} }},
		{"negative limit", func(c *AccessTokenConfig) { l := DefaultLimits(); l.DiscoveryBytes = -1; c.Limits = &l }},
		{"overflow limit", func(c *AccessTokenConfig) { l := DefaultLimits(); l.ErrorBytes = math.MaxInt64; c.Limits = &l }},
		{"event exceeds stream", func(c *AccessTokenConfig) { l := DefaultLimits(); l.StreamBytes = 1; c.Limits = &l }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.change(&cfg)
			p, err := NewWithAccessToken(cfg)
			require.Error(t, err)
			assert.Nil(t, p)
			assert.NotContains(t, err.Error(), "secret")
		})
	}
	_, err := NewWithAccessToken(valid, nil)
	require.Error(t, err)
	for _, id := range []string{"", " ", "a\nb", "a\tb", string([]byte{255})} {
		p, err := NewWithAccessToken(valid)
		require.NoError(t, err)
		_, err = p.LanguageModel(id)
		require.Error(t, err)
	}
}

func TestProvider_ImmutableConfigurationAndRegistry(t *testing.T) {
	limits := DefaultLimits()
	headers := http.Header{"x-config": {"original"}, "x-access-token": {"fake"}, "X-Grafana-Id": {"fake-user"}}
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		assert.Equal(t, "https://example.test/prefix%2Fsegment/config", req.URL.String())
		assert.Equal(t, "original", req.Header.Get("X-Config"))
		assert.Equal(t, []string{"token"}, req.Header.Values("X-Access-Token"))
		assert.Empty(t, req.Header.Values("X-Grafana-Id"))
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(discoveryFixture)), Request: req}, nil
	})}
	other := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("option transport unexpectedly selected")
		return nil, errors.New("unexpected")
	})}
	p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test/prefix%2Fsegment///", HTTPClient: client, Headers: headers, Limits: &limits}, WithHTTPClient(other))
	require.NoError(t, err)
	headers.Set("x-config", "mutated")
	limits.DiscoveryBytes = 1
	rows, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, int32(1), calls.Load())
	assert.Nil(t, client.CheckRedirect)
	m, err := p.LanguageModel("grafana/assistant")
	require.NoError(t, err)
	assert.Equal(t, "v4", m.SpecificationVersion())
	assert.Equal(t, "grafana", m.Provider())
	assert.Equal(t, "grafana/assistant", m.ModelID())
	assert.Nil(t, m.SupportedURLs())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = m.DoGenerate(ctx, provider.CallOptions{})
	assert.ErrorIs(t, err, context.Canceled)
	_, err = m.DoStream(ctx, provider.CallOptions{})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), calls.Load())
}

func TestProvider_DiscoveryConcurrencyAndActingUser(t *testing.T) {
	var calls atomic.Int32
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/v1/aisdk/config", r.URL.Path)
		assert.Equal(t, []string{"access-token"}, r.Header.Values("X-Access-Token"))
		assert.True(t, strings.HasPrefix(r.Header.Get("X-Grafana-Id"), "user-"))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, discoveryFixture)
	}, nil)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			rows, err := p.ListModels(WithUserIDToken(context.Background(), fmt.Sprintf("user-%d", i)))
			assert.NoError(t, err)
			assert.Len(t, rows, 2)
		})
	}
	wg.Wait()
	assert.Equal(t, int32(32), calls.Load())
}

func TestCloudAuth_ValidationAndExchange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*CloudAuthConfig)
	}{
		{"CAP", func(c *CloudAuthConfig) { c.CAPToken = " " }},
		{"namespace", func(c *CloudAuthConfig) { c.Namespace = " " }},
		{"audience", func(c *CloudAuthConfig) { c.Audience = " " }},
		{"exchange URL", func(c *CloudAuthConfig) { c.TokenExchangeURL = "https://secret@host" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := CloudAuthConfig{CAPToken: "cap", Namespace: "stacks-1", BaseURL: "https://gateway.test", TokenExchangeURL: "https://auth.test"}
			tc.change(&cfg)
			_, err := NewWithCloudAuth(cfg)
			require.Error(t, err)
		})
	}
	for _, audience := range []string{"", "explicit-audience"} {
		t.Run("audience="+audience, func(t *testing.T) {
			var exchanges, gateway atomic.Int32
			claims, _ := json.Marshal(map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
			token := "eyJhbGciOiJFUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(claims) + ".c2lnbmF0dXJl"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/exchange" {
					exchanges.Add(1)
					assert.Equal(t, "Bearer cap", r.Header.Get("Authorization"))
					var request authn.TokenExchangeRequest
					assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
					assert.Equal(t, "stacks-1", request.Namespace)
					expected := audience
					if expected == "" {
						expected = "ai-sdk"
					}
					assert.Equal(t, []string{expected}, request.Audiences)
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"token": token}})
					return
				}
				gateway.Add(1)
				assert.Equal(t, token, r.Header.Get("X-Access-Token"))
				_, _ = io.WriteString(w, discoveryFixture)
			}))
			defer srv.Close()
			p, err := NewWithCloudAuth(CloudAuthConfig{CAPToken: "cap", Namespace: "stacks-1", Audience: audience, BaseURL: srv.URL + "/api", TokenExchangeURL: srv.URL + "/exchange", HTTPClient: srv.Client()})
			require.NoError(t, err)
			for range 2 {
				rows, err := p.ListModels(context.Background())
				require.NoError(t, err)
				assert.Len(t, rows, 2)
			}
			assert.Equal(t, int32(1), exchanges.Load())
			assert.Equal(t, int32(2), gateway.Load())
		})
	}
}

func TestProvider_CancellationAndRedirect(t *testing.T) {
	t.Run("pre-canceled and invalid user never exchange", func(t *testing.T) {
		p := testProvider(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected network request") }, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := p.ListModels(ctx)
		assert.ErrorIs(t, err, context.Canceled)
		_, err = p.ListModels(WithUserIDToken(context.Background(), "bad\nuser"))
		require.Error(t, err)
	})
	t.Run("cancel response read", func(t *testing.T) {
		started := make(chan struct{})
		p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
		}, nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() { _, err := p.ListModels(ctx); done <- err }()
		<-started
		cancel()
		select {
		case err := <-done:
			assert.ErrorIs(t, err, context.Canceled)
		case <-time.After(2 * time.Second):
			t.Fatal("canceled discovery did not finish")
		}
	})
	t.Run("redirect does not leak auth", func(t *testing.T) {
		var received atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { received.Add(1) }))
		defer target.Close()
		p := testProvider(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }, nil)
		_, err := p.ListModels(context.Background())
		require.Error(t, err)
		assert.Zero(t, received.Load())
	})
}

type exchangeFunc func(context.Context, authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error)

func (f exchangeFunc) Exchange(ctx context.Context, req authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
	return f(ctx, req)
}

func TestCloudAuth_FailureAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *authn.TokenExchangeResponse
		err      error
	}{
		{"failure", nil, errors.New("private exchange response")}, {"nil response", nil, nil}, {"empty token", &authn.TokenExchangeResponse{}, nil}, {"unsafe token", &authn.TokenExchangeResponse{Token: "private\r\ninjection"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testProvider(t, func(http.ResponseWriter, *http.Request) { t.Error("Gateway request after failed auth") }, nil)
			p.exchanger = exchangeFunc(func(context.Context, authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
				return tc.response, tc.err
			})
			_, err := p.ListModels(context.Background())
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private")
			if tc.err != nil {
				assert.NotErrorIs(t, err, tc.err)
				assert.Nil(t, errors.Unwrap(err), "token-service response prose must not escape through the cause")
			}
		})
	}
	t.Run("canceled waiter does not wait for another exchange", func(t *testing.T) {
		p := testProvider(t, func(http.ResponseWriter, *http.Request) { t.Error("Gateway request after cancellation") }, nil)
		started := make(chan struct{})
		var exchanges atomic.Int32
		p.exchanger = exchangeFunc(func(ctx context.Context, _ authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
			exchanges.Add(1)
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		first, cancelFirst := context.WithCancel(context.Background())
		defer cancelFirst()
		doneFirst := make(chan error, 1)
		go func() { _, err := p.ListModels(first); doneFirst <- err }()
		<-started
		second, cancelSecond := context.WithCancel(context.Background())
		doneSecond := make(chan error, 1)
		go func() { _, err := p.ListModels(second); doneSecond <- err }()
		cancelSecond()
		select {
		case err := <-doneSecond:
			assert.ErrorIs(t, err, context.Canceled)
		case <-time.After(time.Second):
			t.Fatal("canceled waiter blocked behind token exchange")
		}
		cancelFirst()
		assert.ErrorIs(t, <-doneFirst, context.Canceled)
		assert.Equal(t, int32(1), exchanges.Load())
	})
}
