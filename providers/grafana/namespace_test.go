package grafana

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/authlib/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAudience = "aigateway.ext.grafana.app"

type exchangeKey struct{ namespace, subject string }

type gatewayCall struct{ namespace, route, tokenNamespace string }

// namespaceGateway serves token exchanges and Gateway routes under /api, and
// records which exchange namespace issued the token each route request sent.
type namespaceGateway struct {
	t         *testing.T
	srv       *httptest.Server
	mu        sync.Mutex
	exchanges map[exchangeKey]int
	issued    map[string]string
	calls     []gatewayCall
}

func newNamespaceGateway(t *testing.T) *namespaceGateway {
	t.Helper()
	g := &namespaceGateway{t: t, exchanges: map[exchangeKey]int{}, issued: map[string]string{}}
	g.srv = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *namespaceGateway) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/exchange" {
		var request authn.TokenExchangeRequest
		assert.NoError(g.t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(g.t, []string{testAudience}, request.Audiences)
		token := exchangedToken(g.t, request.Namespace+"|"+request.SubjectToken)
		g.mu.Lock()
		g.exchanges[exchangeKey{request.Namespace, request.SubjectToken}]++
		g.issued[token] = request.Namespace
		g.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"token": token}})
		return
	}
	namespace, route, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/namespaces/"), "/")
	assert.True(g.t, ok && strings.HasPrefix(r.URL.Path, "/api/namespaces/"), r.URL.Path)
	assert.Len(g.t, r.Header.Values("X-Access-Token"), 1)
	g.mu.Lock()
	tokenNamespace, issued := g.issued[r.Header.Get("X-Access-Token")]
	g.calls = append(g.calls, gatewayCall{namespace, route, tokenNamespace})
	g.mu.Unlock()
	assert.True(g.t, issued, "Gateway request carries an exchanged token")
	switch {
	case route == "config":
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, discoveryFixture)
	case r.Header.Get("ai-language-model-streaming") == "true":
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sseFrame(finishEvent))
	default:
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, unaryFixture)
	}
}

func (g *namespaceGateway) provider(t *testing.T, namespace string) *Provider {
	t.Helper()
	p, err := NewWithTokenExchange(TokenExchangeConfig{
		CAPToken: "cap", Namespace: namespace, Audience: testAudience,
		BaseURL: g.srv.URL + "/api", TokenExchangeURL: g.srv.URL + "/exchange", HTTPClient: g.srv.Client(),
	})
	require.NoError(t, err)
	return p
}

func callEveryOperation(t *testing.T, ctx context.Context, p *Provider) {
	t.Helper()
	rows, err := p.ListModels(ctx)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
	model, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	generated, err := model.DoGenerate(ctx, provider.CallOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, generated.Content)
	streamed, err := model.DoStream(ctx, provider.CallOptions{})
	require.NoError(t, err)
	parts := collectParts(t, streamed)
	require.NotEmpty(t, parts)
	assert.Equal(t, provider.PartFinish, parts[len(parts)-1].Type)
}

func operationCalls(namespace, tokenNamespace string) []gatewayCall {
	return []gatewayCall{{namespace, "config", tokenNamespace}, {namespace, "language-model", tokenNamespace}, {namespace, "language-model", tokenNamespace}}
}

func TestWithNamespace_RoutesAndExchangesPerCall(t *testing.T) {
	t.Run("one provider serves two stacks", func(t *testing.T) {
		g := newNamespaceGateway(t)
		p := g.provider(t, "")
		var want []gatewayCall
		for range 2 {
			for _, namespace := range []string{"stacks-1", "stacks-2"} {
				for _, subject := range []string{"", "subject-token-user"} {
					ctx := WithNamespace(context.Background(), namespace)
					if subject != "" {
						ctx = WithOnBehalfOf(ctx, subject)
					}
					callEveryOperation(t, ctx, p)
					want = append(want, operationCalls(namespace, namespace)...)
				}
			}
		}
		assert.Equal(t, want, g.calls)
		assert.Equal(t, map[exchangeKey]int{
			{"stacks-1", ""}: 1, {"stacks-1", "subject-token-user"}: 1,
			{"stacks-2", ""}: 1, {"stacks-2", "subject-token-user"}: 1,
		}, g.exchanges, "tokens are cached per namespace and subject")
	})
	t.Run("configured namespace is the default", func(t *testing.T) {
		g := newNamespaceGateway(t)
		p := g.provider(t, "stacks-1")
		callEveryOperation(t, context.Background(), p)
		callEveryOperation(t, WithNamespace(context.Background(), "stacks-1"), p)
		assert.Equal(t, append(operationCalls("stacks-1", "stacks-1"), operationCalls("stacks-1", "stacks-1")...), g.calls)
		assert.Equal(t, map[exchangeKey]int{{"stacks-1", ""}: 1}, g.exchanges)
	})
	t.Run("wildcard exchange routes per call", func(t *testing.T) {
		g := newNamespaceGateway(t)
		p := g.provider(t, "*")
		callEveryOperation(t, WithNamespace(context.Background(), "stacks-1"), p)
		callEveryOperation(t, WithNamespace(context.Background(), "stacks-2"), p)
		assert.Equal(t, append(operationCalls("stacks-1", "*"), operationCalls("stacks-2", "*")...), g.calls)
		assert.Equal(t, map[exchangeKey]int{{"*", ""}: 1}, g.exchanges)
	})
}

func TestWithNamespace_RefusedBeforeIO(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		t.Error("unexpected network request")
		return nil, errors.New("unexpected network request")
	})}
	exchange := func(namespace string) func() (*Provider, error) {
		return func() (*Provider, error) {
			return NewWithTokenExchange(TokenExchangeConfig{CAPToken: "cap", Namespace: namespace, Audience: testAudience, BaseURL: "https://gateway.invalid", TokenExchangeURL: "https://exchange.invalid", HTTPClient: transport})
		}
	}
	for _, tc := range []struct {
		name      string
		new       func() (*Provider, error)
		namespace *string
	}{
		{"empty configured namespace without call namespace", exchange(""), nil},
		{"wildcard without call namespace", exchange("*"), nil},
		{"conflicting call namespace", exchange("stacks-1"), new("stacks-2")},
		{"empty call namespace", exchange(""), new("")},
		{"wildcard call namespace", exchange(""), new("*")},
		{"uppercase call namespace", exchange(""), new("Private-Marker")},
		{"dotted call namespace", exchange(""), new("private.marker")},
		{"path call namespace", exchange(""), new("private/marker")},
		{"traversal call namespace", exchange("*"), new("..")},
		{"whitespace call namespace", exchange(""), new("private marker")},
		{"leading dash call namespace", exchange(""), new("-private-marker")},
		{"long call namespace", exchange(""), new(strings.Repeat("a", 64))},
		{"access token flow", func() (*Provider, error) {
			return NewWithAccessToken(AccessTokenConfig{AccessToken: "access", BaseURL: "https://gateway.invalid/namespaces/stacks-1", HTTPClient: transport})
		}, new("stacks-1")},
		{"service account flow", func() (*Provider, error) {
			return NewWithServiceAccountToken(ServiceAccountConfig{Token: "glsa_token", BaseURL: "https://gateway.invalid/namespaces/stacks-1", HTTPClient: transport})
		}, new("stacks-1")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := tc.new()
			require.NoError(t, err)
			ctx := context.Background()
			if tc.namespace != nil {
				ctx = WithNamespace(ctx, *tc.namespace)
			}
			_, err = p.ListModels(ctx)
			require.Error(t, err)
			assert.NotContains(t, strings.ToLower(err.Error()), "marker")
			model, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			_, err = model.DoGenerate(ctx, provider.CallOptions{})
			require.Error(t, err)
			_, err = model.DoStream(ctx, provider.CallOptions{})
			require.Error(t, err)
		})
	}
	assert.Zero(t, calls.Load())
}

func TestWithNamespace_ConcurrentExchangesPerNamespace(t *testing.T) {
	var gatewayMu sync.Mutex
	sent := map[string][]string{}
	transport := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		namespace, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/namespaces/"), "/")
		gatewayMu.Lock()
		sent[namespace] = append(sent[namespace], r.Header.Get("X-Access-Token"))
		gatewayMu.Unlock()
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(discoveryFixture)), Request: r}, nil
	})}
	p, err := NewWithTokenExchange(TokenExchangeConfig{CAPToken: "cap", Audience: testAudience, BaseURL: "https://gateway.invalid", TokenExchangeURL: "https://exchange.invalid", HTTPClient: transport})
	require.NoError(t, err)
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var mu sync.Mutex
		exchanges := map[string]int{}
		p.exchanger = exchangeFunc(func(_ context.Context, request authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
			mu.Lock()
			exchanges[request.Namespace]++
			mu.Unlock()
			<-release
			return &authn.TokenExchangeResponse{Token: "token-for-" + request.Namespace}, nil
		})
		namespaces := []string{"stacks-1", "stacks-1", "stacks-1", "stacks-2", "stacks-2"}
		errs := make(chan error, len(namespaces))
		for _, namespace := range namespaces {
			go func() {
				_, err := p.ListModels(WithNamespace(context.Background(), namespace))
				errs <- err
			}()
		}
		synctest.Wait()
		mu.Lock()
		assert.Equal(t, map[string]int{"stacks-1": 1, "stacks-2": 1}, exchanges, "callers share one exchange per namespace and never across namespaces")
		mu.Unlock()
		close(release)
		for range namespaces {
			require.NoError(t, <-errs)
		}
	})
	assert.Equal(t, map[string][]string{
		"stacks-1": {"token-for-stacks-1", "token-for-stacks-1", "token-for-stacks-1"},
		"stacks-2": {"token-for-stacks-2", "token-for-stacks-2"},
	}, sent)
}

func TestTokenExchange_HTTPClientCoversExchange(t *testing.T) {
	g := &namespaceGateway{t: t, exchanges: map[exchangeKey]int{}, issued: map[string]string{}}
	g.srv = httptest.NewTLSServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.srv.Close)
	t.Run("trusted certificate authority", func(t *testing.T) {
		callEveryOperation(t, WithNamespace(context.Background(), "stacks-1"), g.provider(t, ""))
		assert.Equal(t, map[exchangeKey]int{{"stacks-1", ""}: 1}, g.exchanges)
	})
	t.Run("untrusted certificate authority", func(t *testing.T) {
		p, err := NewWithTokenExchange(TokenExchangeConfig{CAPToken: "cap", Audience: testAudience, BaseURL: g.srv.URL + "/api", TokenExchangeURL: g.srv.URL + "/exchange", HTTPClient: &http.Client{}})
		require.NoError(t, err)
		before := len(g.calls)
		_, err = p.ListModels(WithNamespace(context.Background(), "stacks-2"))
		require.Error(t, err)
		assert.Len(t, g.calls, before)
		assert.NotContains(t, g.exchanges, exchangeKey{"stacks-2", ""})
	})
	t.Run("exchange redirect is not followed", func(t *testing.T) {
		var received atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { received.Add(1) }))
		defer target.Close()
		redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		}))
		defer redirect.Close()
		p, err := NewWithTokenExchange(TokenExchangeConfig{CAPToken: "cap", Audience: testAudience, BaseURL: redirect.URL + "/api", TokenExchangeURL: redirect.URL + "/exchange", HTTPClient: redirect.Client()})
		require.NoError(t, err)
		_, err = p.ListModels(WithNamespace(context.Background(), "stacks-1"))
		require.Error(t, err)
		assert.Zero(t, received.Load())
	})
}
