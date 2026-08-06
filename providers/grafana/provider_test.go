package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/gateway/providerwire"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/registry"
	"github.com/grafana/authlib/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturedRequest struct {
	Method        string
	Path          string
	ModelID       string
	Streaming     string
	SpecVersion   string
	ContentType   string
	Accept        string
	AccessToken   string
	Authorization string
	UserIDToken   string
	TestHeader    string
	Body          []byte
	CallOptions   provider.CallOptions
}

type fakeHostedEndpoint struct {
	t        *testing.T
	server   *httptest.Server
	mu       sync.Mutex
	requests []capturedRequest
	handlers []http.HandlerFunc
}

func newFakeHostedEndpoint(t *testing.T, handlers ...http.HandlerFunc) *fakeHostedEndpoint {
	t.Helper()
	f := &fakeHostedEndpoint{t: t, handlers: handlers}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeHostedEndpoint) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	opts, err := providerwire.DecodeCallOptions(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	captured := capturedRequest{
		Method:        r.Method,
		Path:          r.URL.Path,
		ModelID:       r.Header.Get(providerwire.HeaderModelID),
		Streaming:     r.Header.Get(providerwire.HeaderStreaming),
		SpecVersion:   r.Header.Get(providerwire.HeaderSpecVersion),
		ContentType:   r.Header.Get("Content-Type"),
		Accept:        r.Header.Get("Accept"),
		AccessToken:   r.Header.Get(accessTokenHeader),
		Authorization: r.Header.Get("Authorization"),
		UserIDToken:   r.Header.Get(userIDHeader),
		TestHeader:    r.Header.Get("X-Test"),
		Body:          body,
		CallOptions:   opts,
	}
	f.mu.Lock()
	f.requests = append(f.requests, captured)
	idx := len(f.requests) - 1
	f.mu.Unlock()

	if !f.validateRequest(w, captured) {
		return
	}
	if idx >= len(f.handlers) {
		http.Error(w, fmt.Sprintf("no handler for request %d", idx+1), http.StatusInternalServerError)
		return
	}
	f.handlers[idx](w, r)
}

func (f *fakeHostedEndpoint) validateRequest(w http.ResponseWriter, req capturedRequest) bool {
	checks := map[string]bool{
		"method":       req.Method == http.MethodPost,
		"path":         req.Path == providerwire.PathLanguageModel,
		"model id":     req.ModelID == "claude-sonnet-4-5-20250929",
		"streaming":    req.Streaming == "true" || req.Streaming == "false",
		"spec version": req.SpecVersion == providerwire.SpecVersionV4,
		"content type": strings.HasPrefix(req.ContentType, providerwire.MIMEJSON),
		"accept":       req.Accept == providerwire.MIMEJSON || req.Accept == providerwire.MIMESSE,
		"access token": req.AccessToken == "access-token",
	}
	for name, ok := range checks {
		if !ok {
			f.t.Errorf("invalid %s in request: %+v", name, req)
			http.Error(w, "invalid "+name, http.StatusBadRequest)
			return false
		}
	}
	return true
}

func (f *fakeHostedEndpoint) URL() string { return f.server.URL }

func (f *fakeHostedEndpoint) Requests() []capturedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]capturedRequest(nil), f.requests...)
}

type fakeTokenExchanger struct {
	mu       sync.Mutex
	token    string
	err      error
	requests []authn.TokenExchangeRequest
}

func (f *fakeTokenExchanger) Exchange(_ context.Context, req authn.TokenExchangeRequest) (*authn.TokenExchangeResponse, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return &authn.TokenExchangeResponse{Token: f.token}, nil
}

func (f *fakeTokenExchanger) Requests() []authn.TokenExchangeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]authn.TokenExchangeRequest(nil), f.requests...)
}

func newTestProvider(t *testing.T, baseURL string, exchanger authn.TokenExchanger, cfgOpts ...func(*CloudAuthConfig)) *Provider {
	t.Helper()
	cfg := CloudAuthConfig{
		Namespace: "stacks-1",
		BaseURL:   baseURL,
	}
	for _, opt := range cfgOpts {
		opt(&cfg)
	}
	normalizedBaseURL, err := normalizeBaseURL(cfg.BaseURL)
	require.NoError(t, err)
	audience := cfg.Audience
	if audience == "" {
		audience = defaultAudience
	}
	return &Provider{
		baseURL:        normalizedBaseURL,
		namespace:      cfg.Namespace,
		audience:       audience,
		httpClient:     configuredHTTPClient(cfg.HTTPClient, nil),
		tokenExchanger: exchanger,
	}
}

func newTestModel(t *testing.T, endpoint *fakeHostedEndpoint, exchanger *fakeTokenExchanger) provider.LanguageModel {
	t.Helper()
	p := newTestProvider(t, endpoint.URL(), exchanger)
	model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
	require.NoError(t, err)
	return model
}

func newAccessTokenProvider(t *testing.T, baseURL string, opts ...Option) *Provider {
	t.Helper()
	p, err := NewWithAccessToken(AccessTokenConfig{
		AccessToken: "access-token",
		BaseURL:     baseURL,
	}, opts...)
	require.NoError(t, err)
	return p
}

func newAccessTokenModel(t *testing.T, endpoint *fakeHostedEndpoint) provider.LanguageModel {
	t.Helper()
	p := newAccessTokenProvider(t, endpoint.URL())
	model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
	require.NoError(t, err)
	return model
}

func testCallOptions() provider.CallOptions {
	maxOutputTokens := 128
	temperature := 0.7
	return provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hello")},
		MaxOutputTokens: &maxOutputTokens,
		Temperature:     &temperature,
		StopSequences:   []string{"stop"},
		Headers:         map[string]string{"X-Test": "true"},
		ProviderOptions: provider.ProviderOptions{"grafana": provider.RawProviderOption{Key: "grafana", Raw: json.RawMessage(`{"trace":true}`)}},
	}
}

func finishPart() provider.StreamPart {
	finish := provider.FinishReason{Unified: provider.FinishReasonStop}
	return provider.StreamPart{
		Type:         provider.PartFinish,
		FinishReason: &finish,
		Usage:        &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(1)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(2)}},
	}
}

func intPtr(v int) *int { return &v }

func streamSuccess(parts ...provider.StreamPart) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", providerwire.MIMESSE)
		w.WriteHeader(http.StatusOK)
		for _, part := range parts {
			_ = providerwire.WriteSSEStreamPartTo(w, part)
		}
	}
}

func generateSuccess(result *provider.GenerateResult) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		body, err := providerwire.EncodeGenerateResult(result)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", providerwire.MIMEJSON)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}

func errorResponse(apiErr *provider.APICallError) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_ = providerwire.WriteErrorResponse(w, apiErr)
	}
}

func malformedErrorResponse(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", providerwire.MIMEJSON)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func malformedStream() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", providerwire.MIMESSE)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: {bad-json}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
}

func blockingStream(started chan<- struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", providerwire.MIMESSE)
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(started)
		<-r.Context().Done()
	}
}

type closeTrackingBody struct {
	*bytes.Reader
	closed chan struct{}
	once   sync.Once
}

func (b *closeTrackingBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

type readErrorBody struct{ err error }

func (b readErrorBody) Read([]byte) (int, error) { return 0, b.err }
func (b readErrorBody) Close() error             { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func retryableAPIError(message string) *provider.APICallError {
	retryable := true
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:     message,
		StatusCode:  http.StatusTooManyRequests,
		IsRetryable: &retryable,
	})
}

func nonRetryableAPIError(message string) *provider.APICallError {
	retryable := false
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:     message,
		StatusCode:  http.StatusBadRequest,
		IsRetryable: &retryable,
	})
}

func TestNewWithCloudAuth_ValidationAndDefaults(t *testing.T) {
	t.Run("validates required fields", func(t *testing.T) {
		base := CloudAuthConfig{CAPToken: "cap", TokenExchangeURL: "https://auth.example.test", Namespace: "ns", BaseURL: "https://ai.example.test"}
		cases := []struct {
			name   string
			mutate func(*CloudAuthConfig)
		}{
			{name: "CAPToken", mutate: func(cfg *CloudAuthConfig) { cfg.CAPToken = "" }},
			{name: "TokenExchangeURL", mutate: func(cfg *CloudAuthConfig) { cfg.TokenExchangeURL = "" }},
			{name: "Namespace", mutate: func(cfg *CloudAuthConfig) { cfg.Namespace = "" }},
			{name: "BaseURL", mutate: func(cfg *CloudAuthConfig) { cfg.BaseURL = "" }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := base
				tc.mutate(&cfg)
				_, err := NewWithCloudAuth(cfg)
				require.Error(t, err)
			})
		}
	})

	t.Run("defaults audience and normalizes base URL", func(t *testing.T) {
		exchanger := &fakeTokenExchanger{token: "access-token"}
		endpoint := newFakeHostedEndpoint(t, generateSuccess(&provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}))
		p := newTestProvider(t, endpoint.URL()+"/", exchanger)
		model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)
		_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		reqs := exchanger.Requests()
		require.Len(t, reqs, 1)
		assert.Equal(t, "stacks-1", reqs[0].Namespace)
		assert.Equal(t, []string{defaultAudience}, reqs[0].Audiences)
	})

	t.Run("rejects invalid base URLs", func(t *testing.T) {
		cases := []string{
			"ftp://ai.example.test",
			"https://ai.example.test/provider?region=us",
			"https://ai.example.test/provider#fragment",
		}
		for _, rawURL := range cases {
			t.Run(rawURL, func(t *testing.T) {
				_, err := NewWithCloudAuth(CloudAuthConfig{
					CAPToken:         "cap-token",
					TokenExchangeURL: "https://auth.example.test/exchange",
					Namespace:        "stacks-1",
					BaseURL:          rawURL,
				})
				require.Error(t, err)
			})
		}
	})
}

func TestNewWithAccessToken_ValidationAndDefaults(t *testing.T) {
	t.Run("validates required fields", func(t *testing.T) {
		base := AccessTokenConfig{AccessToken: "access-token", BaseURL: "https://ai.example.test"}
		cases := []struct {
			name   string
			mutate func(*AccessTokenConfig)
		}{
			{name: "AccessToken", mutate: func(cfg *AccessTokenConfig) { cfg.AccessToken = "" }},
			{name: "BaseURL", mutate: func(cfg *AccessTokenConfig) { cfg.BaseURL = "" }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := base
				tc.mutate(&cfg)
				_, err := NewWithAccessToken(cfg)
				require.Error(t, err)
			})
		}
	})

	t.Run("rejects invalid base URLs", func(t *testing.T) {
		cases := []string{
			"ftp://ai.example.test",
			"https://ai.example.test/provider?region=us",
			"https://ai.example.test/provider#fragment",
		}
		for _, rawURL := range cases {
			t.Run(rawURL, func(t *testing.T) {
				_, err := NewWithAccessToken(AccessTokenConfig{
					AccessToken: "access-token",
					BaseURL:     rawURL,
				})
				require.Error(t, err)
			})
		}
	})

	t.Run("normalizes base URL and defaults HTTP client", func(t *testing.T) {
		p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "access-token", BaseURL: "https://ai.example.test/provider/"})
		require.NoError(t, err)
		assert.Equal(t, "https://ai.example.test/provider", p.baseURL)
		assert.Same(t, http.DefaultClient, p.httpClient)
	})

	t.Run("uses HTTP client option when config omits client", func(t *testing.T) {
		client := &http.Client{}
		p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "access-token", BaseURL: "https://ai.example.test"}, WithHTTPClient(client))
		require.NoError(t, err)
		assert.Same(t, client, p.httpClient)
	})
}

func TestNewWithAccessToken_NoExchangeHTTP(t *testing.T) {
	endpoint := newFakeHostedEndpoint(t,
		generateSuccess(&provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}),
		streamSuccess(finishPart()),
	)
	baseURL, err := url.Parse(endpoint.URL())
	require.NoError(t, err)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != baseURL.Host {
			return nil, fmt.Errorf("unexpected request outside provider-wire endpoint: %s", req.URL.String())
		}
		return http.DefaultTransport.RoundTrip(req)
	})}
	p, err := NewWithAccessToken(AccessTokenConfig{
		AccessToken: "access-token",
		BaseURL:     endpoint.URL(),
		HTTPClient:  client,
	})
	require.NoError(t, err)
	model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
	require.NoError(t, err)

	_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	stream, err := model.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	_ = collectStream(stream.Stream)

	assert.Len(t, endpoint.Requests(), 2)
}

func TestNewWithAccessToken_HeaderPropagation(t *testing.T) {
	endpoint := newFakeHostedEndpoint(t,
		generateSuccess(&provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}),
		generateSuccess(&provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}),
	)
	model := newAccessTokenModel(t, endpoint)

	_, err := model.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	_, err = model.DoGenerate(WithUserIDToken(context.Background(), "id-token"), provider.CallOptions{})
	require.NoError(t, err)

	requests := endpoint.Requests()
	require.Len(t, requests, 2)
	assert.Equal(t, "access-token", requests[0].AccessToken)
	assert.Empty(t, requests[0].UserIDToken)
	assert.Equal(t, "access-token", requests[1].AccessToken)
	assert.Equal(t, "id-token", requests[1].UserIDToken)
}

func TestNewWithAccessToken_RegistryAndModelMetadata(t *testing.T) {
	p := newAccessTokenProvider(t, "https://ai.example.test")
	model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
	require.NoError(t, err)

	assert.Equal(t, "v4", model.SpecificationVersion())
	assert.Equal(t, providerName, model.Provider())
	assert.Equal(t, "claude-sonnet-4-5-20250929", model.ModelID())
	assert.Nil(t, model.SupportedURLs())

	reg := registry.NewProviderRegistry(map[string]registry.Provider{"grafana": p})
	resolved, err := reg.LanguageModel("grafana:claude-sonnet-4-5-20250929")
	require.NoError(t, err)
	assert.Equal(t, providerName, resolved.Provider())
	assert.Equal(t, "claude-sonnet-4-5-20250929", resolved.ModelID())

	wrapped := registry.NewProviderRegistry(
		map[string]registry.Provider{"grafana": p},
		registry.WithLanguageModelMiddleware(middleware.Middleware{
			OverrideProvider: func(provider.LanguageModel) string { return "wrapped-grafana" },
		}),
	)
	wrappedModel, err := wrapped.LanguageModel("grafana:claude-sonnet-4-5-20250929")
	require.NoError(t, err)
	assert.Equal(t, "wrapped-grafana", wrappedModel.Provider())
}

func TestNewWithAccessToken_StreamAndGenerateParity(t *testing.T) {
	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "text-1"},
		{Type: provider.PartTextDelta, ID: "text-1", Delta: "hello"},
		{Type: provider.PartTextEnd, ID: "text-1"},
		finishPart(),
	}
	expectedGenerate := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hello"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage:        provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(1)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(2)}},
	}
	endpoint := newFakeHostedEndpoint(t, streamSuccess(parts...), generateSuccess(expectedGenerate))
	model := newAccessTokenModel(t, endpoint)

	stream, err := model.DoStream(context.Background(), testCallOptions())
	require.NoError(t, err)
	assert.Equal(t, parts, collectStream(stream.Stream))

	gotGenerate, err := model.DoGenerate(context.Background(), testCallOptions())
	require.NoError(t, err)
	assert.Equal(t, expectedGenerate.Content, gotGenerate.Content)
	assert.Equal(t, expectedGenerate.FinishReason, gotGenerate.FinishReason)
	assert.Equal(t, expectedGenerate.Usage, gotGenerate.Usage)
}

func TestDoGenerate_JoinsBaseURLPath(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		generateSuccess(&provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}})(w, r)
	}))
	t.Cleanup(server.Close)

	p := newTestProvider(t, server.URL+"/ai-sdk/", &fakeTokenExchanger{token: "access-token"})
	model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
	require.NoError(t, err)

	_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assert.Equal(t, "/ai-sdk/language-model", gotPath)
}

func TestModel_MetadataAndRegistry(t *testing.T) {
	endpoint := newFakeHostedEndpoint(t)
	exchanger := &fakeTokenExchanger{token: "access-token"}
	p := newTestProvider(t, endpoint.URL(), exchanger)
	model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
	require.NoError(t, err)

	assert.Equal(t, "v4", model.SpecificationVersion())
	assert.Equal(t, providerName, model.Provider())
	assert.Equal(t, "claude-sonnet-4-5-20250929", model.ModelID())
	assert.Nil(t, model.SupportedURLs())

	reg := registry.NewProviderRegistry(map[string]registry.Provider{"grafana": p})
	resolved, err := reg.LanguageModel("grafana:claude-sonnet-4-5-20250929")
	require.NoError(t, err)
	assert.Equal(t, providerName, resolved.Provider())
	assert.Equal(t, "claude-sonnet-4-5-20250929", resolved.ModelID())
}

func TestModel_MiddlewareComposition(t *testing.T) {
	endpoint := newFakeHostedEndpoint(t)
	exchanger := &fakeTokenExchanger{token: "access-token"}
	p := newTestProvider(t, endpoint.URL(), exchanger)
	reg := registry.NewProviderRegistry(
		map[string]registry.Provider{"grafana": p},
		registry.WithLanguageModelMiddleware(middleware.Middleware{
			OverrideProvider: func(provider.LanguageModel) string { return "wrapped-grafana" },
		}),
	)
	model, err := reg.LanguageModel("grafana:claude-sonnet-4-5-20250929")
	require.NoError(t, err)
	assert.Equal(t, "wrapped-grafana", model.Provider())
}

func TestDoStream_Success(t *testing.T) {
	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "text-1"},
		{Type: provider.PartTextDelta, ID: "text-1", Delta: "hello"},
		{Type: provider.PartTextEnd, ID: "text-1"},
		finishPart(),
	}
	endpoint := newFakeHostedEndpoint(t, streamSuccess(parts...))
	exchanger := &fakeTokenExchanger{token: "access-token"}
	model := newTestModel(t, endpoint, exchanger)

	ctx := WithUserIDToken(context.Background(), "id-token")
	stream, err := model.DoStream(ctx, testCallOptions())
	require.NoError(t, err)

	var got []provider.StreamPart
	for part := range stream.Stream {
		got = append(got, part)
	}
	assert.Equal(t, parts, got)

	requests := endpoint.Requests()
	require.Len(t, requests, 1)
	assert.Equal(t, "true", requests[0].Streaming)
	assert.Equal(t, providerwire.MIMESSE, requests[0].Accept)
	assert.Equal(t, "id-token", requests[0].UserIDToken)
	assert.Equal(t, "true", requests[0].TestHeader)
	assert.Equal(t, testCallOptions(), requests[0].CallOptions)

	exchangeRequests := exchanger.Requests()
	require.Len(t, exchangeRequests, 1)
	assert.Equal(t, []string{defaultAudience}, exchangeRequests[0].Audiences)
}

func TestDoGenerate_Success(t *testing.T) {
	responseMeta := provider.ResponseMetadata{ID: "msg_1", ModelID: "claude-sonnet-4-5-20250929", Timestamp: time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)}
	expected := &provider.GenerateResult{
		Content:          []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hello"}},
		FinishReason:     provider.FinishReason{Unified: provider.FinishReasonStop},
		Usage:            provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(1)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(2)}},
		ProviderMetadata: provider.ProviderMetadata{"grafana": json.RawMessage(`{"from":"server"}`)},
		Request:          &provider.RequestMetadata{Body: json.RawMessage(`{"server":"request"}`)},
		Response: &provider.GenerateResponse{
			ResponseMetadata: responseMeta,
			Headers:          map[string]string{"X-Model-Trace": "model-trace"},
			Body:             json.RawMessage(`{"model":"metadata"}`),
		},
	}
	endpoint := newFakeHostedEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Trace", "transport-trace")
		generateSuccess(expected)(w, r)
	})
	exchanger := &fakeTokenExchanger{token: "access-token"}
	model := newTestModel(t, endpoint, exchanger)

	got, err := model.DoGenerate(context.Background(), testCallOptions())
	require.NoError(t, err)
	assert.Equal(t, expected.Content, got.Content)
	assert.Equal(t, expected.FinishReason, got.FinishReason)
	assert.Equal(t, expected.Usage, got.Usage)
	assert.Equal(t, expected.ProviderMetadata, got.ProviderMetadata)
	require.NotNil(t, got.Request)
	assert.JSONEq(t, `{"server":"request"}`, string(got.Request.Body))
	require.NotNil(t, got.Response)
	assert.Equal(t, responseMeta, got.Response.ResponseMetadata)
	assert.Equal(t, map[string]string{"X-Model-Trace": "model-trace"}, got.Response.Headers)
	assert.JSONEq(t, `{"model":"metadata"}`, string(got.Response.Body))

	requests := endpoint.Requests()
	require.Len(t, requests, 1)
	assert.Equal(t, "false", requests[0].Streaming)
	assert.Equal(t, providerwire.MIMEJSON, requests[0].Accept)
	assert.Empty(t, requests[0].UserIDToken)
	assert.Equal(t, "true", requests[0].TestHeader)
	assert.Equal(t, testCallOptions(), requests[0].CallOptions)
}

func TestDoGenerate_FillsLocalRequestResponseMetadata(t *testing.T) {
	expected := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hello"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
	}
	endpoint := newFakeHostedEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Trace", "transport-trace")
		generateSuccess(expected)(w, r)
	})
	exchanger := &fakeTokenExchanger{token: "access-token"}
	model := newTestModel(t, endpoint, exchanger)

	got, err := model.DoGenerate(context.Background(), testCallOptions())
	require.NoError(t, err)

	require.NotNil(t, got.Request)
	assert.JSONEq(t, string(endpoint.Requests()[0].Body), string(got.Request.Body))
	require.NotNil(t, got.Response)
	assert.Equal(t, "transport-trace", got.Response.Headers["X-Trace"])
	assert.Contains(t, string(got.Response.Body), "hello")
}

func TestDoStream_FiltersRawChunksByDefault(t *testing.T) {
	raw := provider.StreamPart{Type: provider.PartRaw, RawValue: json.RawMessage(`{"raw":true}`)}
	text := provider.StreamPart{Type: provider.PartTextDelta, Delta: "hello"}
	endpoint := newFakeHostedEndpoint(t, streamSuccess(raw, text, finishPart()))
	exchanger := &fakeTokenExchanger{token: "access-token"}
	model := newTestModel(t, endpoint, exchanger)

	stream, err := model.DoStream(context.Background(), testCallOptions())
	require.NoError(t, err)
	var got []provider.StreamPart
	for part := range stream.Stream {
		got = append(got, part)
	}

	require.Len(t, got, 2)
	assert.Equal(t, provider.PartTextDelta, got[0].Type)
	assert.Equal(t, provider.PartFinish, got[1].Type)
}

func TestDoStream_IncludesRawChunksWhenRequested(t *testing.T) {
	raw := provider.StreamPart{Type: provider.PartRaw, RawValue: json.RawMessage(`{"raw":true}`)}
	opts := testCallOptions()
	opts.IncludeRawChunks = true
	endpoint := newFakeHostedEndpoint(t, streamSuccess(raw, finishPart()))
	exchanger := &fakeTokenExchanger{token: "access-token"}
	model := newTestModel(t, endpoint, exchanger)

	stream, err := model.DoStream(context.Background(), opts)
	require.NoError(t, err)
	var got []provider.StreamPart
	for part := range stream.Stream {
		got = append(got, part)
	}

	require.Len(t, got, 2)
	assert.Equal(t, provider.PartRaw, got[0].Type)
	assert.Equal(t, provider.PartFinish, got[1].Type)
}

func TestHTTPErrorMapping(t *testing.T) {
	t.Run("preserves decoded API call error", func(t *testing.T) {
		apiErr := retryableAPIError("rate limited")
		apiErr.URL = "https://server.example.test/custom"
		apiErr.ResponseHeaders = map[string][]string{"X-Trace": {"abc"}}
		apiErr.ResponseBody = "server body"
		apiErr.Data = json.RawMessage(`{"code":"rate_limit"}`)
		endpoint := newFakeHostedEndpoint(t, errorResponse(apiErr))
		model := newTestModel(t, endpoint, &fakeTokenExchanger{token: "access-token"})

		_, err := model.DoGenerate(context.Background(), provider.CallOptions{})
		var got *provider.APICallError
		require.ErrorAs(t, err, &got)
		assert.Equal(t, apiErr.IsRetryable, got.IsRetryable)
		assert.Equal(t, apiErr.StatusCode, got.StatusCode)
		assert.Equal(t, apiErr.ResponseHeaders, got.ResponseHeaders)
		assert.Equal(t, apiErr.ResponseBody, got.ResponseBody)
		assert.Equal(t, apiErr.URL, got.URL)
		assert.JSONEq(t, string(apiErr.Data), string(got.Data))
	})

	t.Run("synthesizes malformed HTTP error body", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t, malformedErrorResponse(http.StatusTeapot, "not-json"))
		model := newTestModel(t, endpoint, &fakeTokenExchanger{token: "access-token"})

		_, err := model.DoGenerate(context.Background(), provider.CallOptions{})
		var got *provider.APICallError
		require.ErrorAs(t, err, &got)
		assert.Equal(t, http.StatusTeapot, got.StatusCode)
		assert.False(t, got.IsRetryable)
		assert.Equal(t, "not-json", got.ResponseBody)
		assert.Contains(t, got.Message, "decoding error response")
	})

	t.Run("synthesizes retryable transport error", func(t *testing.T) {
		p := newTestProvider(t, "http://127.0.0.1:1", &fakeTokenExchanger{token: "access-token"})
		model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)

		_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
		var got *provider.APICallError
		require.ErrorAs(t, err, &got)
		assert.True(t, got.IsRetryable)
		assert.Contains(t, got.Message, "model call request failed")
	})

	t.Run("synthesizes retryable client timeout", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(100 * time.Millisecond)
			generateSuccess(&provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}})(w, nil)
		})
		p := newTestProvider(t, endpoint.URL(), &fakeTokenExchanger{token: "access-token"}, func(cfg *CloudAuthConfig) {
			cfg.HTTPClient = &http.Client{Timeout: 10 * time.Millisecond}
		})
		model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)

		_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
		var got *provider.APICallError
		require.ErrorAs(t, err, &got)
		assert.True(t, got.IsRetryable)
		assert.Contains(t, got.Message, "model call request failed")
	})

	t.Run("synthesizes retryable generate body read error", func(t *testing.T) {
		p := newTestProvider(t, "https://ai.example.test", &fakeTokenExchanger{token: "access-token"}, func(cfg *CloudAuthConfig) {
			cfg.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"X-Trace": {"abc"}, "Content-Type": {providerwire.MIMEJSON}},
					Body:       readErrorBody{err: io.ErrUnexpectedEOF},
					Request:    req,
				}, nil
			})}
		})
		model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)

		_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
		var got *provider.APICallError
		require.ErrorAs(t, err, &got)
		assert.True(t, got.IsRetryable)
		assert.Equal(t, http.StatusOK, got.StatusCode)
		assert.Equal(t, map[string][]string{"X-Trace": {"abc"}, "Content-Type": {providerwire.MIMEJSON}}, got.ResponseHeaders)
	})

	t.Run("synthesizes non-retryable generate decode error", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", providerwire.MIMEJSON)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "not-json")
		})
		model := newTestModel(t, endpoint, &fakeTokenExchanger{token: "access-token"})

		_, err := model.DoGenerate(context.Background(), provider.CallOptions{})
		var got *provider.APICallError
		require.ErrorAs(t, err, &got)
		assert.False(t, got.IsRetryable)
		assert.Equal(t, http.StatusOK, got.StatusCode)
		assert.Equal(t, "not-json", got.ResponseBody)
	})

	t.Run("categorized error surfaces a GatewayError", func(t *testing.T) {
		cases := []struct {
			name      string
			data      string
			wantType  GatewayErrorType
			wantModel string
		}{
			{
				name:     "rate limit",
				data:     `{"error":{"type":"rate_limit_error","message":"slow down"}}`,
				wantType: GatewayErrorRateLimit,
			},
			{
				name:     "authentication",
				data:     `{"error":{"type":"authentication_error","message":"bad key"}}`,
				wantType: GatewayErrorAuthentication,
			},
			{
				name:      "model not found",
				data:      `{"error":{"type":"not_found_error","modelId":"claude-x"}}`,
				wantType:  GatewayErrorModelNotFound,
				wantModel: "claude-x",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				apiErr := retryableAPIError("boom")
				apiErr.Data = json.RawMessage(tc.data)
				endpoint := newFakeHostedEndpoint(t, errorResponse(apiErr))
				model := newTestModel(t, endpoint, &fakeTokenExchanger{token: "access-token"})

				_, err := model.DoGenerate(context.Background(), provider.CallOptions{})

				var gw *GatewayError
				require.ErrorAs(t, err, &gw)
				assert.Equal(t, tc.wantType, gw.Type)
				assert.Equal(t, tc.wantModel, gw.ModelID)

				// The originating APICallError is still reachable.
				var apiGot *provider.APICallError
				require.ErrorAs(t, err, &apiGot)
				assert.Equal(t, apiErr.StatusCode, apiGot.StatusCode)
				assert.JSONEq(t, tc.data, string(apiGot.Data))
			})
		}
	})

	t.Run("uncategorized error stays a plain APICallError", func(t *testing.T) {
		apiErr := retryableAPIError("rate limited")
		apiErr.Data = json.RawMessage(`{"code":"rate_limit"}`) // no structured type
		endpoint := newFakeHostedEndpoint(t, errorResponse(apiErr))
		model := newTestModel(t, endpoint, &fakeTokenExchanger{token: "access-token"})

		_, err := model.DoGenerate(context.Background(), provider.CallOptions{})

		var gw *GatewayError
		assert.False(t, errors.As(err, &gw))

		var apiGot *provider.APICallError
		require.ErrorAs(t, err, &apiGot)
		assert.JSONEq(t, string(apiErr.Data), string(apiGot.Data))
	})
}

func TestStreamFailureMappingAndCancellation(t *testing.T) {
	t.Run("non-SSE 2xx response returns API call error", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", providerwire.MIMEJSON)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"error":"not a stream"}`)
		})
		model := newTestModel(t, endpoint, &fakeTokenExchanger{token: "access-token"})

		stream, err := model.DoStream(context.Background(), provider.CallOptions{})
		require.Nil(t, stream)
		var got *provider.APICallError
		require.ErrorAs(t, err, &got)
		assert.False(t, got.IsRetryable)
		assert.Equal(t, http.StatusOK, got.StatusCode)
		assert.Equal(t, `{"error":"not a stream"}`, got.ResponseBody)
		assert.Contains(t, got.Message, "expected stream response content type")
	})

	t.Run("malformed stream emits non-retryable PartError", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t, malformedStream())
		model := newTestModel(t, endpoint, &fakeTokenExchanger{token: "access-token"})

		stream, err := model.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		parts := collectStream(stream.Stream)
		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartError, parts[0].Type)
		require.NotNil(t, parts[0].APICallError)
		assert.False(t, parts[0].APICallError.IsRetryable)
	})

	t.Run("stream transport read error is retryable", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Request:    httptest.NewRequest(http.MethodPost, "http://example.test/language-model", nil),
		}

		apiErr := newStreamAPICallError(resp, io.ErrUnexpectedEOF)
		assert.True(t, apiErr.IsRetryable)
	})

	t.Run("context cancellation closes stream", func(t *testing.T) {
		started := make(chan struct{})
		endpoint := newFakeHostedEndpoint(t, blockingStream(started))
		model := newTestModel(t, endpoint, &fakeTokenExchanger{token: "access-token"})
		ctx, cancel := context.WithCancel(context.Background())

		stream, err := model.DoStream(ctx, provider.CallOptions{})
		require.NoError(t, err)
		<-started
		cancel()

		select {
		case _, ok := <-stream.Stream:
			assert.False(t, ok)
		case <-time.After(2 * time.Second):
			t.Fatal("stream did not close after context cancellation")
		}
	})

	t.Run("context cancellation while send is blocked closes body", func(t *testing.T) {
		var buf bytes.Buffer
		for i := 0; i < streamBufferSize+1; i++ {
			require.NoError(t, providerwire.WriteSSEStreamPart(&buf, provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "x"}))
		}

		body := &closeTrackingBody{Reader: bytes.NewReader(buf.Bytes()), closed: make(chan struct{})}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       body,
			Request:    httptest.NewRequest(http.MethodPost, "http://example.test/language-model", nil),
		}
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan provider.StreamPart, streamBufferSize)

		go (&model{}).readStream(ctx, resp, ch, false)
		require.Eventually(t, func() bool { return len(ch) == streamBufferSize }, time.Second, 10*time.Millisecond)
		cancel()

		select {
		case <-body.closed:
		case <-time.After(time.Second):
			t.Fatal("response body was not closed after cancellation under backpressure")
		}
	})
}

func collectStream(ch <-chan provider.StreamPart) []provider.StreamPart {
	var parts []provider.StreamPart
	for part := range ch {
		parts = append(parts, part)
	}
	return parts
}

func TestStreamText_RetrySemantics(t *testing.T) {
	t.Run("retryable HTTP error is retried", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t,
			errorResponse(retryableAPIError("retry me")),
			streamSuccess(
				provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"},
				provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "recovered"},
				provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"},
				finishPart(),
			),
		)
		model := newTestModel(t, endpoint, &fakeTokenExchanger{token: "access-token"})

		result := aisdk.StreamText(context.Background(), model,
			aisdk.WithModelMessages(provider.UserText("hello")),
			aisdk.WithMaxRetries(2),
		)
		for range result.FullStream() {
		}

		assert.NoError(t, result.Err())
		assert.Equal(t, "recovered", result.Text())
		assert.Len(t, endpoint.Requests(), 2)
	})

	t.Run("non-retryable HTTP error is not retried", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t, errorResponse(nonRetryableAPIError("do not retry")))
		model := newTestModel(t, endpoint, &fakeTokenExchanger{token: "access-token"})

		result := aisdk.StreamText(context.Background(), model,
			aisdk.WithModelMessages(provider.UserText("hello")),
			aisdk.WithMaxRetries(2),
		)
		for range result.FullStream() {
		}

		require.Error(t, result.Err())
		assert.Len(t, endpoint.Requests(), 1)
	})
}

func TestAuthBehavior(t *testing.T) {
	t.Run("forwards access token and optional user ID token", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t,
			generateSuccess(&provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}),
			generateSuccess(&provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}),
		)
		model := newTestModel(t, endpoint, &fakeTokenExchanger{token: "access-token"})

		_, err := model.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		_, err = model.DoGenerate(WithUserIDToken(context.Background(), "id-token"), provider.CallOptions{})
		require.NoError(t, err)

		requests := endpoint.Requests()
		require.Len(t, requests, 2)
		assert.Equal(t, "access-token", requests[0].AccessToken)
		assert.Empty(t, requests[0].UserIDToken)
		assert.Equal(t, "access-token", requests[1].AccessToken)
		assert.Equal(t, "id-token", requests[1].UserIDToken)
	})

	t.Run("token exchange failure returns local error before model call", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t, generateSuccess(&provider.GenerateResult{}))
		model := newTestModel(t, endpoint, &fakeTokenExchanger{err: errors.New("auth failed")})

		_, err := model.DoGenerate(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exchanging access token")
		assert.Empty(t, endpoint.Requests())
	})

	t.Run("configured audience override reaches token exchange", func(t *testing.T) {
		exchanger := &fakeTokenExchanger{token: "access-token"}
		endpoint := newFakeHostedEndpoint(t, generateSuccess(&provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}))
		p := newTestProvider(t, endpoint.URL(), exchanger, func(cfg *CloudAuthConfig) { cfg.Audience = "custom-audience" })
		model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)

		_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		requests := exchanger.Requests()
		require.Len(t, requests, 1)
		assert.Equal(t, []string{"custom-audience"}, requests[0].Audiences)
	})
}

func TestNonInjectedAuthlibClientUsesConfig(t *testing.T) {
	var authRequests []capturedRequest
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req authn.TokenExchangeRequest
		_ = json.Unmarshal(body, &req)
		authRequests = append(authRequests, capturedRequest{Authorization: r.Header.Get("Authorization"), Body: body})
		assert.Equal(t, "stacks-1", req.Namespace)
		assert.Equal(t, []string{defaultAudience}, req.Audiences)
		w.Header().Set("Content-Type", providerwire.MIMEJSON)
		_, _ = w.Write([]byte(`{"data":{"token":"access-token"}}`))
	}))
	t.Cleanup(authServer.Close)

	endpoint := newFakeHostedEndpoint(t, generateSuccess(&provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}))
	p, err := NewWithCloudAuth(CloudAuthConfig{
		CAPToken:         "cap-token",
		TokenExchangeURL: authServer.URL,
		Namespace:        "stacks-1",
		BaseURL:          endpoint.URL(),
	})
	require.NoError(t, err)
	model, err := p.LanguageModel("claude-sonnet-4-5-20250929")
	require.NoError(t, err)

	_, err = model.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	require.Len(t, authRequests, 1)
	assert.Equal(t, "Bearer cap-token", authRequests[0].Authorization)
}
