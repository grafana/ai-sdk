package gateway

import (
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

func TestCatalogAvailableModels_ThroughPlatformHost(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	h.createTeamCatalog(t)
	const upstreamMetadata = "sentinel-untrusted-provider-metadata"
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/models", r.URL.Path)
		assert.Equal(t, routeCatalogKey, r.Header.Get("X-Api-Key"))
		_, _ = io.WriteString(w, `{"data":[{"id":"new-model","name":"`+upstreamMetadata+`"},{"id":"prefix-`+routeCatalogKey+`"}],"has_more":false}`)
	}))
	defer upstream.Close()
	useNativeAnthropicURL(t, upstream.URL)
	user := session(h.host.AddSession(types.TypeUser, "listing-user", testNamespace))
	anon := session(h.host.AddSession(types.TypeAnonymous, "listing-anonymous", testNamespace))
	base := h.catalogURL("available-models")
	send := func(method, query, body string, auth credential) (*http.Response, string) {
		request, err := http.NewRequestWithContext(t.Context(), method, base+query, strings.NewReader(body))
		require.NoError(t, err)
		auth(request)
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer func() { _ = response.Body.Close() }()
		b, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response, string(b)
	}

	for _, tc := range []struct {
		name, method, query, body string
		auth                      credential
		status                    int
	}{
		{"missing", http.MethodPost, "", "", user, 400},
		{"empty", http.MethodPost, "?provider=", "", user, 400},
		{"duplicate", http.MethodPost, "?provider=claude&provider=claude", "", user, 400},
		{"extra", http.MethodPost, "?provider=claude&url=sentinel", "", user, 400},
		{"malformed encoding", http.MethodPost, "?provider=claude%zz", "", user, 400},
		{"malformed value", http.MethodPost, "?provider=claude%2Fother", "", user, 400},
		{"unknown", http.MethodPost, "?provider=anthropic", "", user, 404},
		{"draft", http.MethodPost, "?provider=claude", `{"key":"sentinel"}`, user, 400},
		{"denied", http.MethodPost, "?provider=claude", "", anon, 403},
		{"get", http.MethodGet, "?provider=claude", "", user, 405},
		{"head", http.MethodHead, "?provider=claude", "", user, 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, body := send(tc.method, tc.query, tc.body, tc.auth)
			assert.Equal(t, tc.status, response.StatusCode, body)
			assert.NotContains(t, body, "sentinel")
			assert.Zero(t, calls.Load())
		})
	}

	for range 2 {
		response, body := send(http.MethodPost, "?provider=claude", "", user)
		require.Equal(t, http.StatusOK, response.StatusCode, body)
		assert.Equal(t, "no-store", response.Header.Get("Cache-Control"))
		assert.JSONEq(t, `{"status":"partial","modelIds":["new-model"]}`, body)
	}
	assert.EqualValues(t, 2, calls.Load())
	assert.Zero(t, h.anthropic.requests.Load())
	for _, private := range []string{routeCatalogKey, upstreamMetadata, upstream.URL, "new-model"} {
		assert.NotContains(t, h.logs.String(), private)
		assert.NotContains(t, h.metrics(t), private)
	}
	assert.Contains(t, h.logs.String(), `"route":"catalog_available_models"`)
}

func TestCatalogAvailableModels_ParentAndConfiguration(t *testing.T) {
	listings := service.NewCatalogHealthChecker(nil)
	notFound := http.NotFoundHandler()
	handler, err := newHandler(gatewayauth.AccessPolicy{}, newTestTelemetry(t), productionRoutes(
		notFound, notFound, notFound, notFound, catalogHealth(listings), catalogAvailableModels(listings),
	))
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		parent []byte
		keys   resource.DecryptedSecureValues
		status int
	}{
		{"absent", nil, nil, 500},
		{"wrong parent", testCatalogParent(t, func(object map[string]any) { object["kind"] = "Middleware" }), nil, 500},
		{"missing key", testCatalogParent(t, nil), nil, 424},
		{"custom endpoint", testCatalogParent(t, func(object map[string]any) {
			object["spec"].(map[string]any)["providers"] = map[string]any{"claude": map[string]any{"type": "anthropic", "baseURL": "https://example.invalid"}}
		}), resource.DecryptedSecureValues{"claude": "secret-key"}, 424},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := testCatalogInfo(tc.parent)
			info.DecryptedSecureValues = tc.keys
			request := callerRequest(bodyBYOKCaller, "/modelcatalogs/"+testCatalogName+"/available-models?provider=claude", nil)
			request = request.WithContext(resource.WithRouteRequestInfo(request.Context(), info))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, tc.status, response.Code, response.Body.String())
			assert.NotContains(t, response.Body.String(), "secret-key")
			assert.NotContains(t, response.Body.String(), "example.invalid")
		})
	}
}
