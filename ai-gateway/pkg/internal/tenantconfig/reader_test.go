package tenantconfig

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/extensionhost"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/manifestdata"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const (
	readerSentinel = "sentinel-reader-body"
	listPath       = "/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-1/middlewares"
)

func enabled(value bool) *bool { return &value }

func newTestReader(t *testing.T, handler http.HandlerFunc) *Reader {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	reader, err := NewReader(rest.Config{Host: server.URL})
	require.NoError(t, err)
	return reader
}

func writeList(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func TestReader_Read(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pages    map[string]string
		expected []extensionhost.TenantResource
	}{
		{
			name:     "no resources",
			pages:    map[string]string{"": `{"items":[]}`},
			expected: nil,
		},
		{
			name: "two pages sorted by name",
			pages: map[string]string{
				"":   `{"metadata":{"continue":"c1"},"items":[{"metadata":{"name":"b"},"spec":{"type":"b","enabled":true,"config":{"x":"y"}}}]}`,
				"c1": `{"items":[{"metadata":{"name":"a"},"spec":{"type":"a","enabled":false}}]}`,
			},
			expected: []extensionhost.TenantResource{
				{Name: "a", Type: "a", Enabled: enabled(false), Config: json.RawMessage(`{}`)},
				{Name: "b", Type: "b", Enabled: enabled(true), Config: json.RawMessage(`{"x":"y"}`)},
			},
		},
		{
			name:  "numbers keep their literal",
			pages: map[string]string{"": `{"items":[{"metadata":{"name":"a"},"spec":{"type":"a","enabled":true,"config":{ "n" : 9007199254740993, "f": 1.0 }}}]}`},
			expected: []extensionhost.TenantResource{
				{Name: "a", Type: "a", Enabled: enabled(true), Config: json.RawMessage(`{"n":9007199254740993,"f":1.0}`)},
			},
		},
		{
			name:  "null configuration and missing enabled",
			pages: map[string]string{"": `{"items":[{"metadata":{"name":"a"},"spec":{"type":"a","config":null}}]}`},
			expected: []extensionhost.TenantResource{
				{Name: "a", Type: "a", Config: json.RawMessage(`{}`)},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var queries []string
			reader := newTestReader(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, listPath, r.URL.Path)
				assert.Equal(t, "100", r.URL.Query().Get("limit"))
				continuation := r.URL.Query().Get("continue")
				queries = append(queries, continuation)
				writeList(w, tc.pages[continuation])
			})
			resources, err := reader.Read(context.Background(), "stacks-1")
			require.NoError(t, err)
			assert.Equal(t, tc.expected, resources)
			assert.Len(t, queries, len(tc.pages))
		})
	}
}

func TestReader_Failures(t *testing.T) {
	status := func(code int) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = io.WriteString(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"`+readerSentinel+`","code":`+strconv.Itoa(code)+`}`)
		}
	}
	plain := func(code int) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) { http.Error(w, readerSentinel, code) }
	}
	for _, tc := range []struct {
		name     string
		handler  http.HandlerFunc
		expected error
	}{
		{name: "unauthorized", handler: status(http.StatusUnauthorized), expected: extensionhost.ErrReadRefused},
		{name: "forbidden status", handler: status(http.StatusForbidden), expected: extensionhost.ErrReadRefused},
		{name: "forbidden text", handler: plain(http.StatusForbidden), expected: extensionhost.ErrReadRefused},
		{name: "not found", handler: plain(http.StatusNotFound), expected: extensionhost.ErrReadNotFound},
		{name: "server error", handler: status(http.StatusInternalServerError), expected: extensionhost.ErrReadStatus},
		{name: "malformed", handler: func(w http.ResponseWriter, _ *http.Request) { writeList(w, `{"items":[`+readerSentinel) }, expected: extensionhost.ErrReadInvalid},
		{name: "oversized", handler: func(w http.ResponseWriter, _ *http.Request) {
			writeList(w, `{"items":[],"pad":"`+strings.Repeat("x", maxListBytes)+`"}`)
		}, expected: extensionhost.ErrReadInvalid},
		{name: "endless pages", handler: func(w http.ResponseWriter, _ *http.Request) {
			writeList(w, `{"metadata":{"continue":"again"},"items":[]}`)
		}, expected: extensionhost.ErrReadInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := newTestReader(t, tc.handler)
			resources, err := reader.Read(context.Background(), "stacks-1")
			require.ErrorIs(t, err, tc.expected)
			assert.Nil(t, resources)
			assert.NotContains(t, err.Error(), readerSentinel)
		})
	}

	t.Run("timeout", func(t *testing.T) {
		received := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		reader := newTestReader(t, func(http.ResponseWriter, *http.Request) {
			close(received)
			<-release
		})
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-received
			cancel()
		}()
		_, err := reader.Read(ctx, "stacks-1")
		require.ErrorIs(t, err, extensionhost.ErrReadTimeout)
	})
}

// exchanges records the token exchange requests a test host receives.
type exchanges struct {
	mu       sync.Mutex
	requests []authn.TokenExchangeRequest
}

func (e *exchanges) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err == nil {
			var request authn.TokenExchangeRequest
			if json.Unmarshal(body, &request) == nil {
				e.mu.Lock()
				e.requests = append(e.requests, request)
				e.mu.Unlock()
			}
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		next.ServeHTTP(w, r)
	})
}

func (e *exchanges) recorded() []authn.TokenExchangeRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]authn.TokenExchangeRequest(nil), e.requests...)
}

func TestReader_ReadsAsThePluginThroughTokenExchange(t *testing.T) {
	manifest := manifestdata.LocalManifest().ManifestData
	kind := app.ManifestVersionKind{Kind: "Middleware", Plural: "middlewares", Scope: "Namespaced"}
	host, err := platformhost.New(platformhost.Config{Topology: platformhost.TopologyCloud, PluginID: "grafana-aigateway-app", Group: manifest.Group, Version: "v1alpha1", Kinds: []app.ManifestVersionKind{kind}})
	require.NoError(t, err)
	recorded := &exchanges{}
	mux := http.NewServeMux()
	mux.Handle("POST /exchange", recorded.handler(host.TokenExchangeHandler()))
	mux.Handle("/apis/", host.Handler(nil))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	for namespace, spec := range map[string]string{"stacks-1": `{"type":"a","enabled":true,"config":{"n":9007199254740993}}`, "stacks-2": `{"type":"a","enabled":false}`} {
		token, err := host.ServiceToken("seed", namespace)
		require.NoError(t, err)
		request, err := http.NewRequest(http.MethodPost, server.URL+"/apis/"+manifest.Group+"/v1alpha1/namespaces/"+namespace+"/middlewares", strings.NewReader(`{"metadata":{"name":"a"},"spec":`+spec+`}`))
		require.NoError(t, err)
		request.Header.Set("X-Access-Token", token)
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		_ = response.Body.Close()
		require.Equal(t, http.StatusCreated, response.StatusCode)
	}

	kubeConfig, err := k8s.NewTokenExchangeRestConfig(
		k8s.TokenExchangeCredentials{Token: host.AddAccessPolicy("grafana-aigateway-app"), TokenExchangeURL: server.URL + "/exchange"},
		k8s.RemoteServiceTarget{Host: server.URL, Audiences: []string{manifest.Group}},
	)
	require.NoError(t, err)
	reader, err := NewReader(*kubeConfig)
	require.NoError(t, err)

	first, err := reader.Read(context.Background(), "stacks-1")
	require.NoError(t, err)
	assert.Equal(t, []extensionhost.TenantResource{{Name: "a", Type: "a", Enabled: enabled(true), Config: json.RawMessage(`{"n":9007199254740993}`)}}, first)
	second, err := reader.Read(context.Background(), "stacks-2")
	require.NoError(t, err)
	assert.Equal(t, []extensionhost.TenantResource{{Name: "a", Type: "a", Enabled: enabled(false), Config: json.RawMessage(`{}`)}}, second)
	empty, err := reader.Read(context.Background(), "stacks-3")
	require.NoError(t, err)
	assert.Empty(t, empty)

	requests := recorded.recorded()
	require.NotEmpty(t, requests)
	for _, request := range requests {
		assert.Empty(t, request.SubjectToken, "reads without a caller exchange as the plugin")
		assert.Equal(t, "*", request.Namespace)
	}

	t.Run("a caller on the context would be delegated", func(t *testing.T) {
		idToken, err := host.IDToken(types.TypeUser, "u1", "stacks-1")
		require.NoError(t, err)
		ctx := k8s.ContextWithIDToken(context.Background(), idToken)
		before := len(recorded.recorded())
		_, _ = reader.Read(ctx, "stacks-1")
		after := recorded.recorded()
		require.Greater(t, len(after), before)
		assert.NotEmpty(t, after[len(after)-1].SubjectToken, "the cache must therefore never pass a request context")
	})
}
