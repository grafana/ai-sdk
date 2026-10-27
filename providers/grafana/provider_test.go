package grafana

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

const serviceAccountRoutePrefix = "/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-1"

func TestServiceAccount_Requests(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, []string{"Bearer glsa_dummy-token"}, r.Header.Values("Authorization"))
		assert.Empty(t, r.Header.Values("X-Access-Token"))
		assert.Empty(t, r.Header.Values("X-Grafana-Id"))
		assert.Equal(t, "configured", r.Header.Get("X-Custom"))
		switch r.URL.Path {
		case serviceAccountRoutePrefix + "/config":
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, discoveryFixture)
		case serviceAccountRoutePrefix + "/language-model":
			assert.Equal(t, http.MethodPost, r.Method)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.NotContains(t, string(body), "glsa_dummy-token")
			if r.Header.Get("Ai-Language-Model-Streaming") == "true" {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"type\":\"stream-start\"}\n\ndata: [DONE]\n\n")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, unaryFixture)
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	p, err := NewWithServiceAccountToken(ServiceAccountConfig{
		Token: "glsa_dummy-token", BaseURL: srv.URL + serviceAccountRoutePrefix, Headers: http.Header{"X-Custom": {"configured"}},
	})
	require.NoError(t, err)
	rows, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 2)
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Content[0].Text)
	stream, err := m.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
	require.NoError(t, err)
	for range stream.Stream {
	}
	assert.Equal(t, int32(3), requests.Load())
}

func TestServiceAccount_Validation(t *testing.T) {
	valid := ServiceAccountConfig{Token: "glsa_token", BaseURL: "https://example.test" + serviceAccountRoutePrefix}
	for _, tc := range []struct {
		name   string
		change func(*ServiceAccountConfig)
	}{
		{"empty token", func(c *ServiceAccountConfig) { c.Token = "" }},
		{"token whitespace", func(c *ServiceAccountConfig) { c.Token = "glsa_bad token" }},
		{"token injection", func(c *ServiceAccountConfig) { c.Token = "glsa_bad\r\nsecret" }},
		{"token utf8", func(c *ServiceAccountConfig) { c.Token = string([]byte{0xff}) }},
		{"unsafe URL", func(c *ServiceAccountConfig) { c.BaseURL = "https://secret@example.test" }},
		{"reserved config", func(c *ServiceAccountConfig) { c.Headers = http.Header{"authorization": {"private-value"}} }},
		{"invalid limits", func(c *ServiceAccountConfig) { c.Limits = &Limits{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.change(&cfg)
			p, err := NewWithServiceAccountToken(cfg)
			require.Error(t, err)
			assert.Nil(t, p)
			assert.NotContains(t, err.Error(), "private-value")
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestServiceAccount_ConcurrentAndRedirect(t *testing.T) {
	var reached atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer glsa_dummy", r.Header.Get("Authorization"))
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer origin.Close()
	p, err := NewWithServiceAccountToken(ServiceAccountConfig{Token: "glsa_dummy", BaseURL: origin.URL + "/api/v1/aisdk"})
	require.NoError(t, err)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := p.ListModels(context.Background())
			assert.Error(t, err)
		})
	}
	wg.Wait()
	assert.Zero(t, reached.Load())
}

func TestProvider_ImmutableConfigurationAndRegistry(t *testing.T) {
	limits := DefaultLimits()
	headers := http.Header{"x-config": {"original"}}
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

func TestProvider_DiscoveryConcurrency(t *testing.T) {
	var calls atomic.Int32
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/api/v1/aisdk/config", r.URL.Path)
		assert.Equal(t, []string{"access-token"}, r.Header.Values("X-Access-Token"))
		assert.Empty(t, r.Header.Values("X-Grafana-Id"))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, discoveryFixture)
	}, nil)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			rows, err := p.ListModels(context.Background())
			assert.NoError(t, err)
			assert.Len(t, rows, 2)
		})
	}
	wg.Wait()
	assert.Equal(t, int32(32), calls.Load())
}

func TestTokenExchange_ValidationAndExchange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*TokenExchangeConfig)
	}{
		{"CAP", func(c *TokenExchangeConfig) { c.CAPToken = " " }},
		{"namespace", func(c *TokenExchangeConfig) { c.Namespace = " " }},
		{"audience", func(c *TokenExchangeConfig) { c.Audience = " " }},
		{"omitted audience", func(c *TokenExchangeConfig) { c.Audience = "" }},
		{"exchange URL", func(c *TokenExchangeConfig) { c.TokenExchangeURL = "https://secret@host" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := TokenExchangeConfig{CAPToken: "cap", Namespace: "stacks-1", Audience: "grafana-aigateway-app", BaseURL: "https://gateway.test", TokenExchangeURL: "https://auth.test"}
			tc.change(&cfg)
			_, err := NewWithTokenExchange(cfg)
			require.Error(t, err)
		})
	}
	t.Run("exchange", func(t *testing.T) {
		const audience = "grafana-aigateway-app"
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
				assert.Equal(t, []string{audience}, request.Audiences)
				assert.Empty(t, request.SubjectToken)
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"token": token}})
				return
			}
			gateway.Add(1)
			assert.Equal(t, token, r.Header.Get("X-Access-Token"))
			_, _ = io.WriteString(w, discoveryFixture)
		}))
		defer srv.Close()
		p, err := NewWithTokenExchange(TokenExchangeConfig{CAPToken: "cap", Namespace: "stacks-1", Audience: audience, BaseURL: srv.URL + "/api", TokenExchangeURL: srv.URL + "/exchange", HTTPClient: srv.Client()})
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

func TestCloudAuth_InvalidFileNeverAuthenticatesOrSends(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()
	p, err := NewWithTokenExchange(TokenExchangeConfig{
		CAPToken: "cap", Namespace: "stacks-1", Audience: "grafana-aigateway-app", BaseURL: server.URL + "/api",
		TokenExchangeURL: server.URL + "/exchange", HTTPClient: server.Client(),
	})
	require.NoError(t, err)
	model, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	bad := provider.TextDataContent("")
	bad.URL = "https://example.test/file"
	input := provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("text/plain", bad))}}
	_, err = model.DoGenerate(context.Background(), input)
	require.Error(t, err)
	_, err = model.DoStream(context.Background(), input)
	require.Error(t, err)
	assert.Zero(t, calls.Load())
}

func TestProvider_CancellationAndRedirect(t *testing.T) {
	t.Run("pre-canceled and invalid on-behalf-of never exchange", func(t *testing.T) {
		p := testProvider(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected network request") }, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := p.ListModels(ctx)
		assert.ErrorIs(t, err, context.Canceled)
		_, err = p.ListModels(WithOnBehalfOf(context.Background(), "bad\nuser"))
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

func TestTokenExchange_FailureAndCancellation(t *testing.T) {
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
	t.Run("canceled waiter does not wait for a shared exchange", func(t *testing.T) {
		p := testProvider(t, func(http.ResponseWriter, *http.Request) { t.Error("Gateway request after cancellation") }, nil)
		started, finish := make(chan struct{}), make(chan struct{})
		p.exchanger = exchangeFunc(func(context.Context, authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
			close(started)
			<-finish
			return nil, errors.New("exchange failed")
		})
		defer close(finish)
		first, cancelFirst := context.WithCancel(context.Background())
		defer cancelFirst()
		doneFirst := make(chan error, 1)
		go func() { _, err := p.ListModels(first); doneFirst <- err }()
		<-started
		second, cancelSecond := context.WithCancel(context.Background())
		cancelSecond()
		_, err := p.ListModels(second)
		assert.ErrorIs(t, err, context.Canceled)
		cancelFirst()
		assert.ErrorIs(t, <-doneFirst, context.Canceled)
	})
	t.Run("first caller's cancellation does not cancel a shared exchange", func(t *testing.T) {
		p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, discoveryFixture)
		}, nil)
		exchangeCtx, finish := make(chan context.Context, 1), make(chan struct{})
		p.exchanger = exchangeFunc(func(ctx context.Context, _ authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
			select {
			case exchangeCtx <- ctx:
			default:
			}
			<-finish
			return &authn.TokenExchangeResponse{Token: "exchanged-token"}, ctx.Err()
		})
		first, cancelFirst := context.WithCancel(context.Background())
		doneFirst := make(chan error, 1)
		go func() { _, err := p.ListModels(first); doneFirst <- err }()
		shared := <-exchangeCtx
		doneSecond := make(chan error, 1)
		go func() { _, err := p.ListModels(context.Background()); doneSecond <- err }()
		cancelFirst()
		assert.ErrorIs(t, <-doneFirst, context.Canceled)
		assert.NoError(t, shared.Err(), "the shared exchange outlives the first caller")
		close(finish)
		assert.NoError(t, <-doneSecond)
	})
}

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

func TestOnBehalfOf_ExchangesForDifferentSubjectsConcurrently(t *testing.T) {
	subjects := []string{"subject-token-a", "subject-token-b"}
	var arrived sync.WaitGroup
	arrived.Add(len(subjects))
	allArrived := make(chan struct{})
	go func() { arrived.Wait(); close(allArrived) }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/exchange" {
			var request authn.TokenExchangeRequest
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			arrived.Done()
			select {
			case <-allArrived:
			case <-time.After(5 * time.Second):
				http.Error(w, "exchanges were serialized", http.StatusConflict)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"token": exchangedToken(t, request.SubjectToken)}})
			return
		}
		_, _ = io.WriteString(w, discoveryFixture)
	}))
	defer srv.Close()
	p, err := NewWithTokenExchange(TokenExchangeConfig{
		CAPToken: "cap", Namespace: "stacks-1", Audience: "grafana-aigateway-app",
		BaseURL: srv.URL + "/api", TokenExchangeURL: srv.URL + "/exchange", HTTPClient: srv.Client(),
	})
	require.NoError(t, err)

	errs := make(chan error, len(subjects))
	for _, subject := range subjects {
		go func() {
			_, err := p.ListModels(WithOnBehalfOf(context.Background(), subject))
			errs <- err
		}()
	}
	for range subjects {
		require.NoError(t, <-errs)
	}
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
		subject string
	}{
		{"access token flow", func() (*Provider, error) {
			return NewWithAccessToken(AccessTokenConfig{AccessToken: "access", BaseURL: "https://gateway.invalid", HTTPClient: transport})
		}, "subject-token"},
		{"service account flow", func() (*Provider, error) {
			return NewWithServiceAccountToken(ServiceAccountConfig{Token: "glsa_token", BaseURL: "https://gateway.invalid", HTTPClient: transport})
		}, "subject-token"},
		{"wildcard namespace", exchange("*"), "subject-token"},
		{"empty subject", exchange("stacks-1"), ""},
		{"unsafe subject", exchange("stacks-1"), "subject\r\ntoken"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := tc.new()
			require.NoError(t, err)
			_, err = p.ListModels(WithOnBehalfOf(context.Background(), tc.subject))
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "subject-token")
		})
	}
	assert.Zero(t, calls.Load())
}

func TestProvider_ReservedAuthenticationHeaders(t *testing.T) {
	for _, flow := range []struct {
		name string
		new  func(http.Header) (*Provider, error)
	}{
		{"access", func(headers http.Header) (*Provider, error) {
			return NewWithAccessToken(AccessTokenConfig{AccessToken: "dummy-access", BaseURL: "https://gateway.invalid", Headers: headers})
		}},
		{"exchange", func(headers http.Header) (*Provider, error) {
			return NewWithTokenExchange(TokenExchangeConfig{CAPToken: "dummy-cap", Namespace: "stacks-1", Audience: "grafana-aigateway-app", TokenExchangeURL: "https://exchange.invalid", BaseURL: "https://gateway.invalid", Headers: headers})
		}},
		{"service account", func(headers http.Header) (*Provider, error) {
			return NewWithServiceAccountToken(ServiceAccountConfig{Token: "glsa_dummy", BaseURL: "https://gateway.invalid", Headers: headers})
		}},
	} {
		t.Run(flow.name, func(t *testing.T) {
			p, err := flow.new(nil)
			require.NoError(t, err)
			calls := 0
			p.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				t.Error("reserved authentication header reached network")
				return nil, context.Canceled
			})
			model, err := p.LanguageModel("openai/model")
			require.NoError(t, err)
			for _, name := range []string{"Authorization", "aUtHoRiZaTiOn", "X-Access-Token", "x-grafana-id"} {
				for _, value := range []string{"", "dummy-private-marker"} {
					t.Run(name+"/"+value, func(t *testing.T) {
						configured, err := flow.new(http.Header{name: {value}})
						require.Error(t, err)
						assert.Nil(t, configured)
						assert.NotContains(t, err.Error(), "dummy-private-marker")
						opts := provider.CallOptions{Headers: map[string]string{name: value}}
						generated, err := model.DoGenerate(context.Background(), opts)
						require.Error(t, err)
						assert.Nil(t, generated)
						assert.NotContains(t, err.Error(), "dummy-private-marker")
						streamed, err := model.DoStream(context.Background(), opts)
						require.Error(t, err)
						assert.Nil(t, streamed)
						assert.NotContains(t, err.Error(), "dummy-private-marker")
					})
				}
			}
			assert.Zero(t, calls)
		})
	}
}

func TestLanguageModel_RequestSelectorBounds(t *testing.T) {
	client, err := NewWithServiceAccountToken(ServiceAccountConfig{Token: "glsa_dummy", BaseURL: "https://gateway.invalid/api/v1/aisdk"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name, id string
		valid    bool
	}{
		{"configured", "grafana/assistant", true},
		{"native suffix", "openai/native@revision+suffix/part", true},
		{"UTF8 suffix", "anthropic/モデル", true},
		{"exact byte limit", "openai/" + strings.Repeat("x", 2048-len("openai/")), true},
		{"over byte limit", "openai/" + strings.Repeat("x", 2049-len("openai/")), false},
		{"empty", "", false},
		{"whitespace", "openai/not valid", false},
		{"control", "openai/not\nvalid", false},
		{"invalid UTF8", string([]byte{255}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, err := client.LanguageModel(tc.id)
			if !tc.valid {
				require.Error(t, err)
				assert.Nil(t, model)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.id, model.ModelID())
		})
	}
}

func TestNewWithAccessToken_OpaqueStaticCredential(t *testing.T) {
	const key = "opaque._~+/-key=="
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, key, r.Header.Get("X-Access-Token"))
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("X-Scope-OrgID"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, discoveryFixture)
	}))
	defer server.Close()
	client, err := NewWithAccessToken(AccessTokenConfig{AccessToken: key, BaseURL: server.URL})
	require.NoError(t, err)
	models, err := client.ListModels(context.Background())
	require.NoError(t, err)
	assert.Len(t, models, 2)
	assert.Equal(t, 1, calls)
}
