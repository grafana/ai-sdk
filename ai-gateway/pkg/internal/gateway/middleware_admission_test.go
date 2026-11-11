package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/logging"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/tenantconfig"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
	"github.com/grafana/ai-sdk/schema"
)

const (
	middlewareSentinel = "sentinel-middleware-config-value"
	accessSchema       = `{"type":"object","properties":{"rules":{"type":"array","items":{"type":"object","properties":{"models":{"type":"array","items":{"type":"string"}},"limits":{"type":"object","properties":{"rpm":{"type":"integer","minimum":1}},"additionalProperties":false}},"required":["models"],"additionalProperties":false}},"perModel":{"type":"object","additionalProperties":{"type":"object","properties":{"maxTokens":{"type":"integer"}},"additionalProperties":false}},"note":{"type":"string","maxLength":40}},"additionalProperties":false}`
	accessMember       = `{"note":"Answer concisely.","perModel":{"balanced":{"maxTokens":9007199254740993}},"rules":[{"limits":{"rpm":10},"models":["balanced"]}]}`
)

func middlewareURL(base, name string) string {
	url := base + strings.Replace(RoutePrefix(), "{namespace}", testNamespace, 1) + "/middlewares"
	if name != "" {
		url += "/" + name
	}
	return url
}

// middlewareDocument returns a Middleware object named name with the spec of
// type extensionType and the raw JSON spec members, such as `"marker":{}`.
func middlewareDocument(name, extensionType string, enabled bool, members string) string {
	spec := `{"type":` + strconv.Quote(extensionType) + `,"enabled":` + strconv.FormatBool(enabled)
	if members != "" {
		spec += "," + members
	}
	return `{"apiVersion":"aigateway.ext.grafana.app/v1alpha1","kind":"Middleware","metadata":{"name":` + strconv.Quote(name) + `},"spec":` + spec + `}}`
}

func tenantType(t *testing.T, name, configSchema, example string, enabledByDefault bool) tenanttype.Type {
	t.Helper()
	tt := tenanttype.Type{Name: name, EnabledByDefault: enabledByDefault}
	if configSchema != "" {
		compiled, err := schema.SchemaFromJSON(json.RawMessage(configSchema))
		require.NoError(t, err)
		tt.ConfigSchema, tt.Example = compiled, json.RawMessage(example)
	}
	return tt
}

func bindTenant(tt tenanttype.Type, value any) extension.Definition {
	return tt.Bind(tenanttype.Implementation{FailurePolicy: extension.FailSkip, New: func(json.RawMessage, extension.Dependencies) (any, error) { return value, nil }})
}

// middlewareOptions registers a nested tenant type, a tenant type without
// configuration, and an operator extension.
func middlewareOptions(t *testing.T) harnessOptions {
	t.Helper()
	types := []tenanttype.Type{tenantType(t, "model-access", accessSchema, accessMember, false), tenantType(t, "marker", "", "", false)}
	return harnessOptions{
		tenantTypes: types,
		extensions:  []extension.Definition{bindTenant(types[0], gatewayExtension{}), bindTenant(types[1], gatewayExtension{}), gatewayDefinition("audit", gatewayExtension{})},
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

func storedSpec(t *testing.T, body string) map[string]json.RawMessage {
	t.Helper()
	var object struct {
		Spec map[string]json.RawMessage `json:"spec"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &object))
	return object.Spec
}

func TestMiddlewareAdmission_ThroughThePluginProtocol(t *testing.T) {
	admissionLogs := captureAdmissionLogs(t)
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, middlewareOptions(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token := h.serviceToken(t, testInternalService)

	code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), token, middlewareDocument("model-access", "model-access", true, `"modelAccess":`+accessMember))
	require.Equal(t, http.StatusCreated, code, body)
	code, body = sendObject(t, ctx, http.MethodGet, middlewareURL(h.server.URL, "model-access"), token, "")
	require.Equal(t, http.StatusOK, code, body)
	spec := storedSpec(t, body)
	assert.JSONEq(t, accessMember, string(spec["modelAccess"]), "nested objects, arrays of objects and typed maps are kept")
	assert.Contains(t, string(spec["modelAccess"]), "9007199254740993", "integers above 2^53 stay exact")

	code, body = sendObject(t, ctx, http.MethodPut, middlewareURL(h.server.URL, "model-access"), token, middlewareDocument("model-access", "model-access", false, `"modelAccess":{"note":"Other."}`))
	require.Equal(t, http.StatusOK, code, body)
	code, body = sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), token, middlewareDocument("marker", "marker", true, ""))
	require.Equal(t, http.StatusCreated, code, body)

	sentinelMember := `{"note":"` + middlewareSentinel + `"}`
	refused := []struct {
		name   string
		method string
		path   string
		body   string
		reason string
	}{
		{name: "name differs from type", method: http.MethodPost, body: middlewareDocument("other", "model-access", true, `"modelAccess":`+sentinelMember), reason: "metadata.name must equal spec.type"},
		{name: "member of another type", method: http.MethodPut, path: "model-access", body: middlewareDocument("model-access", "model-access", true, `"marker":`+sentinelMember), reason: "spec may set only spec.modelAccess, the member of spec.type"},
		{name: "undeclared spec member", method: http.MethodPut, path: "model-access", body: middlewareDocument("model-access", "model-access", true, `"modelAccess":{},"`+middlewareSentinel+`":1`), reason: "spec may set only spec.modelAccess, the member of spec.type"},
		{name: "unregistered type", method: http.MethodPost, body: middlewareDocument("unknown", "unknown", true, `"unknown":`+sentinelMember), reason: `spec.type \"unknown\" is not a tenant extension of this Gateway`},
		{name: "operator type", method: http.MethodPost, body: middlewareDocument("audit", "audit", true, `"audit":`+sentinelMember), reason: `spec.type \"audit\" is not a tenant extension of this Gateway`},
		{name: "enabled without its member", method: http.MethodPut, path: "model-access", body: middlewareDocument("model-access", "model-access", true, ""), reason: "spec.modelAccess is required when spec.enabled is true"},
		{name: "value failing the schema", method: http.MethodPut, path: "model-access", body: middlewareDocument("model-access", "model-access", true, `"modelAccess":{"note":"`+middlewareSentinel+middlewareSentinel+`"}`), reason: "spec.modelAccess does not match the extension's schema at /note"},
		{name: "nested value failing the schema", method: http.MethodPut, path: "model-access", body: middlewareDocument("model-access", "model-access", true, `"modelAccess":{"rules":[{"models":["`+middlewareSentinel+`"],"limits":{"rpm":0}}]}`), reason: "spec.modelAccess does not match the extension's schema at /rules/0/limits/rpm"},
		{name: "undeclared key", method: http.MethodPut, path: "model-access", body: middlewareDocument("model-access", "model-access", true, `"modelAccess":{"`+middlewareSentinel+`":"`+middlewareSentinel+`"}`), reason: "spec.modelAccess does not match the extension's schema at /"},
		{name: "member for a type without configuration", method: http.MethodPut, path: "marker", body: middlewareDocument("marker", "marker", true, `"marker":`+sentinelMember), reason: `spec.type \"marker\" takes no configuration member`},
		{name: "oversized member", method: http.MethodPut, path: "model-access", body: middlewareDocument("model-access", "model-access", true, `"modelAccess":{"note":"`+strings.Repeat(middlewareSentinel, tenantconfig.MaxMemberBytes/len(middlewareSentinel)+1)+`"}`), reason: "spec.modelAccess exceeds 65536 bytes"},
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

	for _, name := range []string{"model-access", "marker"} {
		code, body = sendObject(t, ctx, http.MethodDelete, middlewareURL(h.server.URL, name), token, "")
		require.Equal(t, http.StatusOK, code, body)
		code, _ = sendObject(t, ctx, http.MethodGet, middlewareURL(h.server.URL, name), token, "")
		assert.Equal(t, http.StatusNotFound, code)
	}
}

func TestMiddlewareAdmission_ShippedTypesRefuseEveryCreate(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), h.serviceToken(t, testInternalService), middlewareDocument("model-access", "model-access", true, `"modelAccess":{}`))
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.Contains(t, body, `middleware: spec.type \"model-access\" is not a tenant extension of this Gateway`)
}

func TestMiddlewareAdmission_AcrossBuilds(t *testing.T) {
	stored := newHarnessWithOptions(t, platformhost.TopologyCloud, middlewareOptions(t))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	storedToken := stored.serviceToken(t, testInternalService)
	code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(stored.server.URL, ""), storedToken, middlewareDocument("model-access", "model-access", true, `"modelAccess":`+accessMember))
	require.Equal(t, http.StatusCreated, code, body)

	rolledBack := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{host: stored.host})
	token := rolledBack.serviceToken(t, testInternalService)
	url := middlewareURL(rolledBack.server.URL, "model-access")
	for _, tc := range []struct {
		name     string
		enabled  bool
		members  string
		wantCode int
	}{
		{name: "keeping it enabled", enabled: true, members: `"modelAccess":` + accessMember, wantCode: http.StatusBadRequest},
		{name: "changing its member", members: `"modelAccess":{"note":"` + middlewareSentinel + `"}`, wantCode: http.StatusBadRequest},
		{name: "changing a number the platform could round", members: `"modelAccess":` + strings.Replace(accessMember, "9007199254740993", "9007199254740992", 1), wantCode: http.StatusBadRequest},
		{name: "disabling it with the member reformatted", members: `"modelAccess": ` + strings.ReplaceAll(accessMember, ",", " , "), wantCode: http.StatusOK},
		{name: "disabling it with the member removed", wantCode: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := sendObject(t, ctx, http.MethodPut, url, token, middlewareDocument("model-access", "model-access", tc.enabled, tc.members))
			assert.Equal(t, tc.wantCode, code, body)
			if tc.wantCode == http.StatusBadRequest {
				assert.Contains(t, body, "an update may only set spec.enabled to false and keep or remove spec.modelAccess")
				assert.NotContains(t, body, middlewareSentinel)
			}
		})
	}

	rolledBack.served.Close()
	code, body = sendObject(t, ctx, http.MethodPut, url, token, middlewareDocument("model-access", "model-access", false, ""))
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
	types := []tenanttype.Type{tenantType(t, "off-by-default", "", "", false), tenantType(t, "on-by-default", "", "", true)}
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{
		tenantTypes: types,
		extensions:  []extension.Definition{bindTenant(types[0], count(&offRuns)), bindTenant(types[1], count(&onRuns))},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token := h.serviceToken(t, testInternalService)
	for name, enabled := range map[string]bool{"off-by-default": true, "on-by-default": false} {
		code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), token, middlewareDocument(name, name, enabled, ""))
		require.Equal(t, http.StatusCreated, code, body)
	}

	code, body := do(t, languageRequest(t, ctx, h.url("language-model"), unaryBody, false, accessToken(token)))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, int32(0), offRuns.Load())
	assert.Equal(t, int32(1), onRuns.Load())
}
