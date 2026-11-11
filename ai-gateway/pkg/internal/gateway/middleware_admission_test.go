package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/logging"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/tenantconfig"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
	"github.com/grafana/ai-sdk/schema"
)

const middlewareSentinel = "sentinel-middleware-config-value"

func middlewareURL(base, name string) string {
	url := base + strings.Replace(RoutePrefix(), "{namespace}", testNamespace, 1) + "/middlewares"
	if name != "" {
		url += "/" + name
	}
	return url
}

func middlewareDocument(t *testing.T, name, extensionType string, enabled bool, config map[string]any) string {
	t.Helper()
	spec := map[string]any{"type": extensionType, "enabled": enabled}
	if config != nil {
		spec["config"] = config
	}
	body, err := json.Marshal(map[string]any{
		"apiVersion": "aigateway.ext.grafana.app/v1alpha1",
		"kind":       "Middleware",
		"metadata":   map[string]any{"name": name},
		"spec":       spec,
	})
	require.NoError(t, err)
	return string(body)
}

func tenantDefinition(t *testing.T, name string, configSchema string, value any, enabledByDefault bool) extension.Definition {
	t.Helper()
	definition := gatewayDefinition(name, value)
	definition.Scope = extension.ScopeTenant
	definition.FailurePolicy = extension.FailSkip
	definition.EnabledByDefault = enabledByDefault
	if configSchema != "" {
		compiled, err := schema.SchemaFromJSON(json.RawMessage(configSchema))
		require.NoError(t, err)
		definition.ConfigSchema = compiled
	}
	return definition
}

func middlewareDefinitions(t *testing.T) []extension.Definition {
	t.Helper()
	return []extension.Definition{
		tenantDefinition(t, "instructions", `{"type":"object","properties":{"instructions":{"type":"string","maxLength":40},"temperature":{"type":"number"},"concise":{"type":"boolean"},"stop":{"type":"array","items":{"type":"string"}}},"additionalProperties":false}`, gatewayExtension{}, false),
		tenantDefinition(t, "marker", "", gatewayExtension{}, false),
		gatewayDefinition("audit", gatewayExtension{}),
	}
}

func captureAdmissionLogs(t *testing.T) *syncBuffer {
	t.Helper()
	logs := &syncBuffer{}
	previous := logging.DefaultLogger
	logging.DefaultLogger = logging.NewSLogLogger(slog.NewJSONHandler(logs, nil))
	t.Cleanup(func() { logging.DefaultLogger = previous })
	return logs
}

func TestMiddlewareAdmission_ThroughThePluginProtocol(t *testing.T) {
	admissionLogs := captureAdmissionLogs(t)
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{extensions: middlewareDefinitions(t)})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token := h.serviceToken(t, testInternalService)
	config := map[string]any{"instructions": "Answer concisely.", "temperature": 0.5, "concise": true, "stop": []any{"END", "STOP"}}

	code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), token, middlewareDocument(t, "instructions", "instructions", true, config))
	require.Equal(t, http.StatusCreated, code, body)
	code, body = sendObject(t, ctx, http.MethodGet, middlewareURL(h.server.URL, "instructions"), token, "")
	require.Equal(t, http.StatusOK, code, body)
	var stored struct {
		Spec struct {
			Type    string         `json:"type"`
			Enabled bool           `json:"enabled"`
			Config  map[string]any `json:"config"`
		} `json:"spec"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &stored))
	assert.Equal(t, "instructions", stored.Spec.Type)
	assert.True(t, stored.Spec.Enabled)
	assert.Equal(t, config, stored.Spec.Config)

	code, body = sendObject(t, ctx, http.MethodPut, middlewareURL(h.server.URL, "instructions"), token, middlewareDocument(t, "instructions", "instructions", false, map[string]any{"instructions": "Other."}))
	require.Equal(t, http.StatusOK, code, body)
	code, body = sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), token, middlewareDocument(t, "marker", "marker", true, nil))
	require.Equal(t, http.StatusCreated, code, body)

	refused := []struct {
		name   string
		method string
		path   string
		body   string
		reason string
	}{
		{name: "name differs from type", method: http.MethodPost, body: middlewareDocument(t, "other", "instructions", true, map[string]any{"instructions": middlewareSentinel}), reason: "metadata.name must equal spec.type"},
		{name: "unregistered type", method: http.MethodPost, body: middlewareDocument(t, "unknown", "unknown", true, map[string]any{"value": middlewareSentinel}), reason: `spec.type \"unknown\" is not a tenant extension of this Gateway`},
		{name: "operator type", method: http.MethodPost, body: middlewareDocument(t, "audit", "audit", true, map[string]any{"value": middlewareSentinel}), reason: `spec.type \"audit\" is not a tenant extension of this Gateway`},
		{name: "value failing the schema", method: http.MethodPut, path: "instructions", body: middlewareDocument(t, "instructions", "instructions", true, map[string]any{"instructions": middlewareSentinel + middlewareSentinel}), reason: "spec.config does not match the extension's schema at /instructions"},
		{name: "undeclared key", method: http.MethodPut, path: "instructions", body: middlewareDocument(t, "instructions", "instructions", true, map[string]any{middlewareSentinel: middlewareSentinel}), reason: "spec.config does not match the extension's schema at /"},
		{name: "config for a schema-less extension", method: http.MethodPut, path: "marker", body: middlewareDocument(t, "marker", "marker", true, map[string]any{"value": middlewareSentinel}), reason: "spec.config is not accepted: the extension takes no configuration"},
		{name: "oversized config", method: http.MethodPut, path: "instructions", body: middlewareDocument(t, "instructions", "instructions", true, map[string]any{"instructions": strings.Repeat(middlewareSentinel, tenantconfig.MaxConfigBytes/len(middlewareSentinel)+1)}), reason: "spec.config exceeds 65536 bytes"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			code, body := sendObject(t, ctx, tc.method, middlewareURL(h.server.URL, tc.path), token, tc.body)
			assert.Equal(t, http.StatusBadRequest, code, body)
			assert.Contains(t, body, "admission hook for Middleware denied the request: middleware: "+tc.reason)
			assert.NotContains(t, body, middlewareSentinel)
		})
	}
	assert.Contains(t, admissionLogs.String(), "validation failed")
	assert.NotContains(t, admissionLogs.String(), middlewareSentinel)
	assert.NotContains(t, h.logs.String(), middlewareSentinel)

	for _, name := range []string{"instructions", "marker"} {
		code, body = sendObject(t, ctx, http.MethodDelete, middlewareURL(h.server.URL, name), token, "")
		require.Equal(t, http.StatusOK, code, body)
		code, _ = sendObject(t, ctx, http.MethodGet, middlewareURL(h.server.URL, name), token, "")
		assert.Equal(t, http.StatusNotFound, code)
	}
}

func TestMiddlewareAdmission_ShippedDefinitionsRefuseEveryCreate(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), h.serviceToken(t, testInternalService), middlewareDocument(t, "instructions", "instructions", true, nil))
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.Contains(t, body, `middleware: spec.type \"instructions\" is not a tenant extension of this Gateway`)
}

func TestMiddlewareAdmission_AcrossBuilds(t *testing.T) {
	stored := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{extensions: middlewareDefinitions(t)})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	config := map[string]any{"instructions": "Answer concisely."}
	code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(stored.server.URL, ""), stored.serviceToken(t, testInternalService), middlewareDocument(t, "instructions", "instructions", true, config))
	require.Equal(t, http.StatusCreated, code, body)

	rolledBack := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{host: stored.host})
	token := rolledBack.serviceToken(t, testInternalService)
	url := middlewareURL(rolledBack.server.URL, "instructions")
	for _, tc := range []struct {
		name     string
		enabled  bool
		config   map[string]any
		wantCode int
	}{
		{name: "keeping it enabled", enabled: true, config: config, wantCode: http.StatusBadRequest},
		{name: "changing its config", config: map[string]any{"instructions": middlewareSentinel}, wantCode: http.StatusBadRequest},
		{name: "disabling it", config: config, wantCode: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := sendObject(t, ctx, http.MethodPut, url, token, middlewareDocument(t, "instructions", "instructions", tc.enabled, tc.config))
			assert.Equal(t, tc.wantCode, code, body)
			if tc.wantCode == http.StatusBadRequest {
				assert.Contains(t, body, "an update may only set spec.enabled to false and keep spec.config")
				assert.NotContains(t, body, middlewareSentinel)
			}
		})
	}

	rolledBack.served.Close()
	code, body = sendObject(t, ctx, http.MethodPut, url, token, middlewareDocument(t, "instructions", "instructions", false, config))
	assert.Equal(t, http.StatusInternalServerError, code, body)
	assert.Contains(t, body, "admission hook for Middleware failed")
	code, body = sendObject(t, ctx, http.MethodDelete, url, token, "")
	require.Equal(t, http.StatusOK, code, body)
	code, _ = sendObject(t, ctx, http.MethodGet, url, token, "")
	assert.Equal(t, http.StatusNotFound, code)
}

func TestMiddleware_StoredResourcesHaveNoRuntimeEffect(t *testing.T) {
	var offRuns, onRuns atomic.Int32
	count := func(runs *atomic.Int32) gatewayExtension {
		return gatewayExtension{admit: func() error {
			runs.Add(1)
			return nil
		}}
	}
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{extensions: []extension.Definition{
		tenantDefinition(t, "off-by-default", "", count(&offRuns), false),
		tenantDefinition(t, "on-by-default", "", count(&onRuns), true),
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token := h.serviceToken(t, testInternalService)
	for name, enabled := range map[string]bool{"off-by-default": true, "on-by-default": false} {
		code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), token, middlewareDocument(t, name, name, enabled, nil))
		require.Equal(t, http.StatusCreated, code, body)
	}

	code, body := do(t, languageRequest(t, ctx, h.url("language-model"), unaryBody, false, accessToken(token)))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, int32(0), offRuns.Load())
	assert.Equal(t, int32(1), onRuns.Load())
}
