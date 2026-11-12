package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/simple"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/manifestdata"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const catalogKey = "sentinel-catalog-provider-key"

func catalogURL(base, name string) string {
	url := base + strings.Replace(RoutePrefix(), "{namespace}", testNamespace, 1) + "/modelcatalogs"
	if name != "" {
		url += "/" + name
	}
	return url
}

func catalogDocument(t *testing.T, providers map[string]any, secure map[string]any) string {
	t.Helper()
	models := map[string]any{}
	for name := range providers {
		models["model-"+name] = map[string]any{"name": "Model", "primary": map[string]any{"provider": name, "model": "native"}}
	}
	object := map[string]any{
		"apiVersion": "aigateway.ext.grafana.app/v1alpha1",
		"kind":       "ModelCatalog",
		"metadata":   map[string]any{"name": "default"},
		"spec":       map[string]any{"providers": providers, "models": models},
	}
	if secure != nil {
		object["secure"] = secure
	}
	body, err := json.Marshal(object)
	require.NoError(t, err)
	return string(body)
}

func anthropicAt(endpoint string) map[string]any {
	return map[string]any{"anthropic": map[string]any{"type": "anthropic", "baseURL": endpoint}}
}

func sendCatalog(t *testing.T, ctx context.Context, method, url, token, body string) (int, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("X-Access-Token", token)
	request.Header.Set("Content-Type", "application/json")
	return do(t, request)
}

func TestProvider_ManagesTheManifestKinds(t *testing.T) {
	data := manifestdata.LocalManifest().ManifestData
	for _, tc := range []struct {
		name string
		gw   func(t *testing.T) *Gateway
	}{
		{name: "running", gw: func(t *testing.T) *Gateway { return newHarness(t, platformhost.TopologyCloud).gateway }},
		{name: "idle", gw: func(t *testing.T) *Gateway {
			gw, err := New(Options{LookupEnv: lookup(map[string]string{})})
			require.NoError(t, err)
			require.True(t, gw.Idle())
			return gw
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := tc.gw(t).Provider()
			built, err := provider.NewApp(app.Config{ManifestData: *provider.Manifest().ManifestData})
			require.NoError(t, err)
			application, ok := built.(*simple.App)
			require.True(t, ok)
			require.NoError(t, application.ValidateManifest(*data))
			require.Len(t, application.ManagedKinds(), 1)
			assert.Equal(t, "ModelCatalog", application.ManagedKinds()[0].Kind())
		})
	}
}

func TestCatalogAdmission_ThroughThePluginProtocol(t *testing.T) {
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{env: map[string]string{"GF_PLUGIN_CUSTOM_ENDPOINT_ALLOW": "10.20.0.0/16", "GF_PLUGIN_CUSTOM_ENDPOINT_DENY": "denied.example"}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token := h.serviceToken(t, testInternalService)
	providers := map[string]any{"anthropic": map[string]any{"type": "anthropic"}}

	code, body := sendCatalog(t, ctx, http.MethodPost, catalogURL(h.server.URL, ""), token, catalogDocument(t, providers, map[string]any{"anthropic": map[string]any{"create": catalogKey}}))
	require.Equal(t, http.StatusCreated, code, body)
	assert.NotContains(t, body, catalogKey)
	var created struct {
		Secure map[string]map[string]any `json:"secure"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &created))
	name, _ := created.Secure["anthropic"]["name"].(string)
	require.NotEmpty(t, name)
	assert.Equal(t, map[string]any{"name": name}, created.Secure["anthropic"])
	stored, ok := h.host.SecureValue(testNamespace, name)
	require.True(t, ok)
	assert.Equal(t, catalogKey, stored)

	code, body = sendCatalog(t, ctx, http.MethodPut, catalogURL(h.server.URL, "default"), token, catalogDocument(t, providers, nil))
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, name)

	refused := []struct {
		name   string
		method string
		path   string
		body   string
		reason string
	}{
		{name: "plain HTTP base URL", method: http.MethodPut, path: "default", reason: "providers.anthropic.baseURL must use https", body: catalogDocument(t, anthropicAt("http://backend.example"), nil)},
		{name: "private base URL", method: http.MethodPut, path: "default", reason: "providers.anthropic.baseURL is refused by the egress policy", body: catalogDocument(t, anthropicAt("https://10.0.0.5"), nil)},
		{name: "denied base URL host", method: http.MethodPut, path: "default", reason: "providers.anthropic.baseURL is refused by the egress policy", body: catalogDocument(t, anthropicAt("https://denied.example"), nil)},
		{name: "removing a used key", method: http.MethodPut, path: "default", reason: "providers.anthropic requires the secure value secure.anthropic", body: catalogDocument(t, providers, map[string]any{"anthropic": map[string]any{"remove": true}})},
		{name: "another secret by name", method: http.MethodPut, path: "default", reason: "secure.anthropic must not reference a secret by name", body: catalogDocument(t, providers, map[string]any{"anthropic": map[string]any{"name": "shared"}})},
		{name: "provider without a key", method: http.MethodPut, path: "default", reason: "providers.openai requires the secure value secure.openai", body: catalogDocument(t, map[string]any{"anthropic": map[string]any{"type": "anthropic"}, "openai": map[string]any{"type": "openai"}}, map[string]any{"anthropic": map[string]any{"create": catalogKey}})},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			code, body := sendCatalog(t, ctx, tc.method, catalogURL(h.server.URL, tc.path), token, tc.body)
			assert.Equal(t, http.StatusBadRequest, code, body)
			assert.Contains(t, body, "admission hook for ModelCatalog denied the request: modelcatalog: "+tc.reason)
			assert.NotContains(t, body, catalogKey)
		})
	}
	assert.NotContains(t, h.logs.String(), catalogKey)

	for name, endpoint := range map[string]string{"public": "https://anthropic-proxy.example.com", "allowed private": "https://10.20.0.5:8443"} {
		t.Run("accepts a "+name+" base URL", func(t *testing.T) {
			code, body := sendCatalog(t, ctx, http.MethodPut, catalogURL(h.server.URL, "default"), token, catalogDocument(t, anthropicAt(endpoint), nil))
			assert.Equal(t, http.StatusOK, code, body)
		})
	}

	code, body = sendCatalog(t, ctx, http.MethodGet, catalogURL(h.server.URL, "default"), token, "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Contains(t, body, name)
	code, body = sendCatalog(t, ctx, http.MethodDelete, catalogURL(h.server.URL, "default"), token, "")
	require.Equal(t, http.StatusOK, code, body)
	_, ok = h.host.SecureValue(testNamespace, name)
	assert.False(t, ok)
}

func TestPluginProcess_ValidatesCatalogsWithoutLoggingKeys(t *testing.T) {
	p := launchPlugin(t, buildPlugin(t), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token, err := p.host.ServiceToken(testInternalService, testNamespace)
	require.NoError(t, err)
	providers := map[string]any{"anthropic": map[string]any{"type": "anthropic"}}

	code, body := sendCatalog(t, ctx, http.MethodPost, catalogURL(p.server.URL, ""), token, catalogDocument(t, map[string]any{"anthropic": map[string]any{"type": "anthropic", "baseURL": "http://backend.invalid"}}, map[string]any{"anthropic": map[string]any{"create": catalogKey}}))
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.NotContains(t, body, catalogKey)
	code, body = sendCatalog(t, ctx, http.MethodPost, catalogURL(p.server.URL, ""), token, catalogDocument(t, providers, map[string]any{"anthropic": map[string]any{"create": catalogKey}}))
	assert.Equal(t, http.StatusCreated, code, body)
	assert.NotContains(t, body, catalogKey)

	p.process.Stop()
	assert.Contains(t, p.logs.String(), "validation failed")
	assert.NotContains(t, p.logs.String(), catalogKey)
	assert.NotContains(t, p.raw.String(), catalogKey)
}
