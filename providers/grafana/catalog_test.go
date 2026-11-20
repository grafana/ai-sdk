package grafana

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/authlib/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithCatalog_RoutesAndExchangesPerCall(t *testing.T) {
	const subject = "user-token"
	tokens := map[exchangeKey]string{
		{"stacks-1", subject}: exchangedToken(t, "stack-1-user"),
		{"stacks-2", subject}: exchangedToken(t, "stack-2-user"),
		{"*", ""}:             exchangedToken(t, "wildcard-service"),
	}
	exchanges := map[exchangeKey]int{}
	var requests []*http.Request
	var bodies []string
	transport := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		media, body := "application/json", unaryFixture
		status := http.StatusOK
		if r.URL.Host == "exchange.test" {
			var exchange authn.TokenExchangeRequest
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&exchange))
			assert.Equal(t, []string{testAudience}, exchange.Audiences)
			key := exchangeKey{exchange.Namespace, exchange.SubjectToken}
			exchanges[key]++
			body = `{"data":{"token":"` + tokens[key] + `"}}`
		} else {
			requests = append(requests, r)
			var payload []byte
			if r.Body != nil {
				var err error
				payload, err = io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.NoError(t, r.Body.Close())
			}
			bodies = append(bodies, string(payload))
			switch {
			case strings.Contains(r.URL.Path, "/modelcatalogs/Other"):
				status, body = http.StatusNotFound, `{"kind":"Status","status":"Failure","reason":"NotFound","code":404}`
			case r.Method == http.MethodGet:
				body = discoveryFixture
			case r.Header.Get("ai-language-model-streaming") == "true":
				media, body = "text/event-stream", sseFrame(finishEvent)
			}
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {media}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	cfg := TokenExchangeConfig{CAPToken: "cap", Audience: testAudience, TokenExchangeURL: "https://exchange.test/exchange", BaseURL: "https://gateway.test/grafana/api/", HTTPClient: transport}
	p, err := NewWithTokenExchange(cfg)
	require.NoError(t, err)
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	user := WithOnBehalfOf(context.Background(), subject)
	stack1 := WithNamespace(user, "stacks-1")
	selected := WithCatalog(stack1, "team")
	rows, err := p.ListModels(selected)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "assistant", rows[0].ID)
	opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("Hello")}}
	_, err = m.DoGenerate(selected, opts)
	require.NoError(t, err)
	stream, err := m.DoStream(WithNamespace(WithCatalog(user, "team"), "stacks-2"), opts)
	require.NoError(t, err)
	parts := collectParts(t, stream)
	require.Len(t, parts, 1)
	assert.Equal(t, provider.PartFinish, parts[0].Type)

	missing := WithCatalog(selected, "Other #?%2F")
	_, err = p.ListModels(missing)
	var discoveryError *provider.APICallError
	require.ErrorAs(t, err, &discoveryError)
	assert.Equal(t, http.StatusNotFound, discoveryError.StatusCode)
	_, err = m.DoGenerate(missing, opts)
	var modelError *provider.APICallError
	require.ErrorAs(t, err, &modelError)
	assert.Equal(t, http.StatusNotFound, modelError.StatusCode)

	_, err = p.ListModels(stack1)
	require.NoError(t, err)
	_, err = m.DoGenerate(stack1, opts)
	require.NoError(t, err)

	cfg.Namespace = "stacks-1"
	fixed, err := NewWithTokenExchange(cfg)
	require.NoError(t, err)
	for _, ctx := range []context.Context{user, stack1} {
		_, err := fixed.ListModels(WithCatalog(ctx, "team"))
		require.NoError(t, err)
	}
	cfg.Namespace = "*"
	wildcard, err := NewWithTokenExchange(cfg)
	require.NoError(t, err)
	_, err = wildcard.ListModels(WithCatalog(WithNamespace(context.Background(), "stacks-2"), "team"))
	require.NoError(t, err)

	want := []struct {
		method, path, streaming, accept string
		key                             exchangeKey
	}{
		{"GET", "/namespaces/stacks-1/modelcatalogs/team/config", "", "", exchangeKey{"stacks-1", subject}},
		{"POST", "/namespaces/stacks-1/modelcatalogs/team/language-model", "false", "application/json", exchangeKey{"stacks-1", subject}},
		{"POST", "/namespaces/stacks-2/modelcatalogs/team/language-model", "true", "text/event-stream", exchangeKey{"stacks-2", subject}},
		{"GET", "/namespaces/stacks-1/modelcatalogs/Other%20%23%3F%252F/config", "", "", exchangeKey{"stacks-1", subject}},
		{"POST", "/namespaces/stacks-1/modelcatalogs/Other%20%23%3F%252F/language-model", "false", "application/json", exchangeKey{"stacks-1", subject}},
		{"GET", "/namespaces/stacks-1/config", "", "", exchangeKey{"stacks-1", subject}},
		{"POST", "/namespaces/stacks-1/language-model", "false", "application/json", exchangeKey{"stacks-1", subject}},
		{"GET", "/namespaces/stacks-1/modelcatalogs/team/config", "", "", exchangeKey{"stacks-1", subject}},
		{"GET", "/namespaces/stacks-1/modelcatalogs/team/config", "", "", exchangeKey{"stacks-1", subject}},
		{"GET", "/namespaces/stacks-2/modelcatalogs/team/config", "", "", exchangeKey{"*", ""}},
	}
	require.Len(t, requests, len(want))
	for i, tc := range want {
		r := requests[i]
		assert.Equal(t, tc.method, r.Method)
		assert.Equal(t, "/grafana/api"+tc.path, r.URL.RequestURI())
		assert.Equal(t, []string{tokens[tc.key]}, r.Header.Values("X-Access-Token"))
		assert.Empty(t, r.Header.Values("Authorization"))
		assert.Empty(t, r.Header.Values("X-Grafana-Id"))
		if tc.streaming != "" {
			assert.Equal(t, tc.streaming, r.Header.Get("ai-language-model-streaming"))
			assert.Equal(t, tc.accept, r.Header.Get("Accept"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "4", r.Header.Get("ai-language-model-specification-version"))
			assert.Equal(t, "assistant", r.Header.Get("ai-language-model-id"))
			assert.JSONEq(t, `{"prompt":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`, bodies[i])
		}
	}
	assert.Equal(t, map[exchangeKey]int{{"stacks-1", subject}: 2, {"stacks-2", subject}: 1, {"*", ""}: 1}, exchanges)
}

func TestWithCatalog_RefusedBeforeIO(t *testing.T) {
	var calls atomic.Int32
	transport := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("unexpected network request")
	})}
	cfg := TokenExchangeConfig{CAPToken: "cap", Namespace: "stacks-1", Audience: testAudience, BaseURL: "https://gateway.test/api", TokenExchangeURL: "https://exchange.test/exchange", HTTPClient: transport}
	fixed, err := NewWithTokenExchange(cfg)
	require.NoError(t, err)
	cfg.Namespace = ""
	dynamic, err := NewWithTokenExchange(cfg)
	require.NoError(t, err)
	cfg.Namespace = "*"
	wildcard, err := NewWithTokenExchange(cfg)
	require.NoError(t, err)
	catalogBaseURL := cfg.BaseURL + "/namespaces/stacks-1/modelcatalogs/team"
	access, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "access", BaseURL: catalogBaseURL, HTTPClient: transport})
	require.NoError(t, err)
	service, err := NewWithServiceAccountToken(ServiceAccountConfig{Token: "service", BaseURL: catalogBaseURL, HTTPClient: transport})
	require.NoError(t, err)
	for _, p := range []*Provider{access, service} {
		req, err := p.request(context.Background(), http.MethodGet, "/config")
		require.NoError(t, err)
		assert.Equal(t, catalogBaseURL+"/config", req.URL.String())
	}

	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		p       *Provider
		ctx     context.Context
		catalog string
		wantErr string
	}{
		{"empty name", fixed, ctx, "", "grafana: invalid catalog name"},
		{"dot", fixed, ctx, ".", "grafana: invalid catalog name"},
		{"dot dot", fixed, ctx, "..", "grafana: invalid catalog name"},
		{"slash", fixed, ctx, "private/name", "grafana: invalid catalog name"},
		{"conflicting namespace", fixed, WithNamespace(ctx, "stacks-2"), "team", "grafana: call namespace conflicts with the configured namespace"},
		{"missing namespace", dynamic, ctx, "team", "grafana: call namespace required"},
		{"wildcard without namespace", wildcard, ctx, "team", "grafana: call namespace required"},
		{"wildcard with subject", wildcard, WithOnBehalfOf(WithNamespace(ctx, "stacks-1"), "user-token"), "team", "grafana: on-behalf-of calls require a concrete namespace"},
		{"access token", access, ctx, "team", "grafana: call catalog requires token exchange"},
		{"service account", service, ctx, "team", "grafana: call catalog requires token exchange"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := WithCatalog(tc.ctx, tc.catalog)
			_, err := tc.p.ListModels(ctx)
			require.EqualError(t, err, tc.wantErr)
			m, err := tc.p.LanguageModel("assistant")
			require.NoError(t, err)
			_, err = m.DoGenerate(ctx, provider.CallOptions{})
			require.EqualError(t, err, tc.wantErr)
			_, err = m.DoStream(ctx, provider.CallOptions{})
			require.EqualError(t, err, tc.wantErr)
		})
	}
	assert.Zero(t, calls.Load())
}
