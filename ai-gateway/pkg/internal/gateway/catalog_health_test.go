package gateway

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/authlib/types"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/service"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

func TestCatalogHealth_ThroughPlatformHost(t *testing.T) {
	const providerError = "sentinel-untrusted-provider-error"
	h := newHarness(t, platformhost.TopologyCloud)
	h.createTeamCatalog(t)
	var status, calls, redirects atomic.Int32
	var block atomic.Bool
	entered, release := make(chan struct{}, 1), make(chan struct{})
	status.Store(http.StatusOK)
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirects.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer destination.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if block.Load() {
			entered <- struct{}{}
			<-release
		}
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/models", r.URL.Path)
		assert.Equal(t, routeCatalogKey, r.Header.Get("X-Api-Key"))
		code := int(status.Load())
		if code != http.StatusOK {
			w.Header().Set("Location", destination.URL)
			w.WriteHeader(code)
			_, _ = fmt.Fprint(w, providerError, routeCatalogKey)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"claude-sonnet"}],"has_more":false}`)
	}))
	defer upstream.Close()
	useNativeAnthropicURL(t, upstream.URL)
	user := session(h.host.AddSession(types.TypeUser, "health-user", testNamespace))
	anon := session(h.host.AddSession(types.TypeAnonymous, "health-anonymous", testNamespace))
	send := func(method, url, body string, auth credential) (int, string) {
		request, err := http.NewRequest(method, url, strings.NewReader(body))
		require.NoError(t, err)
		auth(request)
		result, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer func() { _ = result.Body.Close() }()
		if result.StatusCode == http.StatusOK {
			assert.Equal(t, "no-store", result.Header.Get("Cache-Control"))
		}
		b, err := io.ReadAll(result.Body)
		require.NoError(t, err)
		return result.StatusCode, string(b)
	}
	t.Run("checks only the stored target using its decrypted key", func(t *testing.T) {
		code, body := send(http.MethodPost, h.catalogURL("health"), "", user)
		require.Equal(t, http.StatusOK, code, body)
		assert.JSONEq(t, `{"providers":{"claude":"ok"},"models":{"team-model":{"primary":"ok","fallback":[]}}}`, body)
		assert.Zero(t, h.anthropic.requests.Load(), "no operator catalog requests")
	})
	t.Run("does not forward redirects or upstream errors", func(t *testing.T) {
		for _, tc := range []struct {
			code   int32
			status string
		}{{301, "refused"}, {401, "invalidKey"}, {429, "refused"}, {503, "unreachable"}} {
			status.Store(tc.code)
			code, body := send(http.MethodPost, h.catalogURL("health"), "", user)
			require.Equal(t, http.StatusOK, code, body)
			assert.JSONEq(t, fmt.Sprintf(`{"providers":{"claude":%q},"models":{"team-model":{"primary":"notChecked","fallback":[]}}}`, tc.status), body)
		}
		assert.Zero(t, redirects.Load())
		assert.EqualValues(t, 5, calls.Load(), "no application retries")
		for _, private := range []string{providerError, routeCatalogKey, destination.URL} {
			assert.NotContains(t, h.logs.String(), private)
			assert.NotContains(t, h.metrics(t), private)
		}
		assert.Contains(t, h.logs.String(), `"route":"catalog_health"`)
	})
	t.Run("refuses before probing", func(t *testing.T) {
		before := calls.Load()
		for _, tc := range []struct {
			method, suffix, body string
			auth                 credential
			want                 int
		}{
			{http.MethodGet, "", "", user, http.StatusMethodNotAllowed},
			{http.MethodHead, "", "", user, http.StatusMethodNotAllowed},
			{http.MethodPost, "", `{"secret":"draft"}`, user, http.StatusBadRequest},
			{http.MethodPost, "?endpoint=ignored", "", user, http.StatusBadRequest},
			{http.MethodPost, "", "", anon, http.StatusForbidden},
		} {
			code, body := send(tc.method, h.catalogURL("health")+tc.suffix, tc.body, tc.auth)
			assert.Equal(t, tc.want, code, body)
			assert.NotContains(t, body, "draft")
		}
		code, body := send(http.MethodPost, h.url("modelcatalogs/missing/health"), "", user)
		assert.Equal(t, http.StatusNotFound, code, body)
		assert.Equal(t, before, calls.Load())
	})
	t.Run("second check is refused while the first runs", func(t *testing.T) {
		block.Store(true)
		status.Store(http.StatusOK)
		defer close(release)
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, h.catalogURL("health"), nil)
		require.NoError(t, err)
		user(request)
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			response, err := http.DefaultClient.Do(request)
			if assert.NoError(t, err) {
				defer func() { _ = response.Body.Close() }()
				assert.Equal(t, http.StatusOK, response.StatusCode)
			}
		}()
		select {
		case <-entered:
		case <-finished:
			t.Error("check finished without reaching the provider")
			return
		}
		code, _ := send(http.MethodPost, h.catalogURL("health"), "", user)
		assert.Equal(t, http.StatusTooManyRequests, code)
		release <- struct{}{}
		<-finished
	})
}

func TestCatalogHealth_ParentAndConfiguration(t *testing.T) {
	checker := service.NewCatalogHealthChecker(nil)
	handler, err := newHandler(gatewayauth.AccessPolicy{}, newTestTelemetry(t), productionRoutes(
		http.NotFoundHandler(), http.NotFoundHandler(), http.NotFoundHandler(), http.NotFoundHandler(), catalogHealth(checker),
	))
	require.NoError(t, err)
	for _, tc := range []struct {
		name string
		info *resource.RouteRequestInfo
		want int
	}{
		{"absent parent", testCatalogInfo(nil), http.StatusInternalServerError},
		{"wrong parent", testCatalogInfo(testCatalogParent(t, func(object map[string]any) { object["kind"] = "Middleware" })), http.StatusInternalServerError},
		{"missing key", &resource.RouteRequestInfo{FullIdentifier: testCatalogInfo(nil).FullIdentifier, Parent: testCatalogParent(t, nil)}, http.StatusFailedDependency},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := callerRequest(bodyBYOKCaller, http.MethodPost, "/modelcatalogs/"+testCatalogName+"/health", nil)
			request = request.WithContext(resource.WithRouteRequestInfo(request.Context(), tc.info))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, tc.want, response.Code, response.Body.String())
			assert.NotContains(t, response.Body.String(), "catalog-key")
		})
	}
}
