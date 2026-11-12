package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/authlib/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/logging"
	"k8s.io/client-go/rest"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/tenantconfig"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
	"github.com/grafana/ai-sdk/schema"
)

const (
	middlewareSentinel = "sentinel-middleware-config-value"
	accessSchema       = `{"type":"object","properties":{"rules":{"type":"array","items":{"type":"object","properties":{"models":{"type":"array","items":{"type":"string"}},"limits":{"type":"object","properties":{"rpm":{"type":"integer","minimum":1}},"additionalProperties":false}},"required":["models"],"additionalProperties":false}},"perModel":{"type":"object","additionalProperties":{"type":"object","properties":{"maxTokens":{"type":"integer","minimum":1}},"additionalProperties":false}},"note":{"type":"string","maxLength":40}},"additionalProperties":false}`
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
		{name: "typed map key", method: http.MethodPut, path: "model-access", body: middlewareDocument("model-access", "model-access", true, `"modelAccess":{"perModel":{"`+middlewareSentinel+`":{"maxTokens":0}}}`), reason: "spec.modelAccess does not match the extension's schema at /perModel/*/maxTokens"},
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

// namespaceRecorder is a tenant extension that records, per admit, the
// call's namespace and the extension and configuration the call carries.
type namespaceRecorder struct {
	name  string
	calls *sync.Map
}

func (r namespaceRecorder) Admit(_ context.Context, call *extension.Call) error {
	config, _ := call.Config.(json.RawMessage)
	r.calls.Store(call.Namespace+" "+r.name+string(config), true)
	return nil
}

const recordingSchema = `{"type":"object","properties":{"v":{"type":"string"},"n":{"type":"integer"},"limits":{"type":"object","properties":{"rpm":{"type":"integer"}},"additionalProperties":false}},"additionalProperties":false}`

// recordingTenant returns a public tenant type with a nested schema and its
// definition, whose instance records each call's configuration.
func recordingTenant(t *testing.T, name string, calls *sync.Map, enabledByDefault bool) (tenanttype.Type, extension.Definition) {
	t.Helper()
	tenant := tenantType(t, name, recordingSchema, `{}`, enabledByDefault)
	return tenant, tenant.Bind(tenanttype.Implementation{FailurePolicy: extension.FailSkip, New: func(json.RawMessage, extension.Dependencies) (any, error) {
		return namespaceRecorder{name: name, calls: calls}, nil
	}})
}

func namespacedURL(base, namespace, route string) string {
	return base + strings.Replace(RoutePrefix(), "{namespace}", namespace, 1) + "/" + route
}

func (h *harness) namespaceToken(t *testing.T, namespace string) string {
	t.Helper()
	token, err := h.host.ServiceToken(testInternalService, namespace)
	require.NoError(t, err)
	return token
}

func TestTenantConfiguration_AppliesEachNamespacesResources(t *testing.T) {
	calls := &sync.Map{}
	configuredType, configured := recordingTenant(t, "configured-access", calls, false)
	defaultType, defaultOn := recordingTenant(t, "default-on", calls, true)
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{tenantTypes: []tenanttype.Type{configuredType, defaultType}, extensions: []extension.Definition{configured, defaultOn}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resources := map[string][]string{
		"stacks-1": {middlewareDocument("configured-access", "configured-access", true, `"configuredAccess":{"v":"one","n":9007199254740993,"limits":{"rpm":10}}`)},
		"stacks-2": {middlewareDocument("configured-access", "configured-access", true, `"configuredAccess":{"v":"two"}`), middlewareDocument("default-on", "default-on", false, "")},
	}
	for namespace, documents := range resources {
		for _, document := range documents {
			code, body := sendObject(t, ctx, http.MethodPost, namespacedURL(h.server.URL, namespace, "middlewares"), h.namespaceToken(t, namespace), document)
			require.Equal(t, http.StatusCreated, code, body)
		}
	}

	user, err := h.host.IDToken(types.TypeUser, "u1", "stacks-1")
	require.NoError(t, err)
	delegated, err := h.host.Exchange(ctx, testInternalService, delegation(user))
	require.NoError(t, err)
	for namespace, token := range map[string]string{"stacks-1": delegated, "stacks-2": h.namespaceToken(t, "stacks-2"), "stacks-3": h.namespaceToken(t, "stacks-3")} {
		code, body := do(t, languageRequest(t, ctx, namespacedURL(h.server.URL, namespace, "language-model"), unaryBody, false, accessToken(token)))
		require.Equal(t, http.StatusOK, code, body)
	}

	var applied []string
	calls.Range(func(entry, _ any) bool {
		applied = append(applied, entry.(string))
		return true
	})
	assert.ElementsMatch(t, []string{
		`stacks-1 configured-access{"limits":{"rpm":10},"n":9007199254740993,"v":"one"}`,
		`stacks-1 default-on{}`,
		`stacks-2 configured-access{"v":"two"}`,
		`stacks-3 default-on{}`,
	}, applied, "each call applies its own namespace's member, nested, with numbers kept exact")

	subjects := h.exchanges.recorded()
	require.NotEmpty(t, subjects, "reads go through the plugin's token exchange")
	for _, subject := range subjects {
		assert.Empty(t, subject, "reads are made as the plugin, even for a call on behalf of a user")
	}
	assert.Contains(t, h.logs.String(), `"msg":"gateway plugin API access","available":true`)
}

func TestTenantConfiguration_NoTenantDefinitionsReadNothing(t *testing.T) {
	h := newHarness(t, platformhost.TopologyCloud)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, body := do(t, languageRequest(t, ctx, h.url("language-model"), unaryBody, false, accessToken(h.serviceToken(t, testInternalService))))
	require.Equal(t, http.StatusOK, code, body)
	assert.Empty(t, h.exchanges.recorded())
	assert.Equal(t, 1, strings.Count(h.logs.String(), `"msg":"gateway plugin API access","available":true`))
	assert.NotContains(t, h.logs.String(), "tenant configuration is not applied")
}

func TestTenantConfiguration_WithoutAPIAccess(t *testing.T) {
	var offRuns, onRuns atomic.Int32
	count := func(runs *atomic.Int32) gatewayExtension {
		return gatewayExtension{admit: func() error {
			runs.Add(1)
			return nil
		}}
	}
	tenantTypes := []tenanttype.Type{tenantType(t, "off-by-default", "", "", false), tenantType(t, "on-by-default", "", "", true)}
	options := harnessOptions{tenantTypes: tenantTypes, extensions: []extension.Definition{bindTenant(tenantTypes[0], count(&offRuns)), bindTenant(tenantTypes[1], count(&onRuns))}}
	withAccess := newHarnessWithOptions(t, platformhost.TopologyCloud, options)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token := withAccess.serviceToken(t, testInternalService)
	for name, enabled := range map[string]bool{"off-by-default": true, "on-by-default": false} {
		code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(withAccess.server.URL, ""), token, middlewareDocument(name, name, enabled, ""))
		require.Equal(t, http.StatusCreated, code, body)
	}

	options.host, options.noAPIAccess = withAccess.host, true
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, options)
	token = h.serviceToken(t, testInternalService)
	for range 2 {
		code, body := do(t, languageRequest(t, ctx, h.url("language-model"), unaryBody, false, accessToken(token)))
		require.Equal(t, http.StatusOK, code, body)
	}
	assert.Equal(t, int32(0), offRuns.Load(), "stored resources are not applied")
	assert.Equal(t, int32(2), onRuns.Load(), "tenant extensions keep their defaults")
	assert.Empty(t, h.exchanges.recorded())
	logs := h.logs.String()
	assert.Equal(t, 1, strings.Count(logs, "gateway tenant configuration is not applied"))
	assert.Contains(t, logs, `"msg":"gateway plugin API access","available":false`)

	code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), token, middlewareDocument("off-by-default", "off-by-default", true, ""))
	assert.Equal(t, http.StatusBadRequest, code, body)
	assert.Contains(t, body, "middleware: this Gateway cannot read Middleware resources on this host, so stored configuration would not be applied")
	code, body = sendObject(t, ctx, http.MethodPut, middlewareURL(h.server.URL, "on-by-default"), token, middlewareDocument("on-by-default", "on-by-default", false, ""))
	assert.Equal(t, http.StatusBadRequest, code, body, "even a disable-only update is refused")
	code, body = sendObject(t, ctx, http.MethodDelete, middlewareURL(h.server.URL, "off-by-default"), token, "")
	assert.Equal(t, http.StatusOK, code, body, "deletes are not reviewed")
}

func markerOptions(t *testing.T) harnessOptions {
	t.Helper()
	marker := tenantType(t, "marker", "", "", false)
	return harnessOptions{tenantTypes: []tenanttype.Type{marker}, extensions: []extension.Definition{bindTenant(marker, gatewayExtension{})}}
}

func TestProvider_DecidesAPIAccessOnce(t *testing.T) {
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, markerOptions(t))
	provider := h.gateway.Provider()
	_, err := provider.NewApp(app.Config{ManifestData: *provider.Manifest().ManifestData})
	require.NoError(t, err)
	logs := h.logs.String()
	assert.Equal(t, 1, strings.Count(logs, `"msg":"gateway plugin API access"`), "a later build without API access changes nothing")
	assert.NotContains(t, logs, "tenant configuration is not applied")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), h.serviceToken(t, testInternalService), middlewareDocument("marker", "marker", true, ""))
	assert.Equal(t, http.StatusCreated, code, body)
}

func TestProvider_CachesTheTenantConnectionError(t *testing.T) {
	options := markerOptions(t)
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, harnessOptions{noAPIAccess: true})
	gw, err := New(Options{LookupEnv: lookup(map[string]string{"GRAFANA_JWKS": mustJWKS(t, h.host), "GF_PLUGIN_ACCESS_MODE": "self-hosted", "GF_PLUGIN_MODEL_CATALOG": modelCatalog(h.anthropic.server.URL, false), "ANTHROPIC_SECRET": providerSecret}), Logger: slog.New(slog.NewJSONHandler(&syncBuffer{}, nil)), Registerer: prometheus.NewRegistry(), Extensions: options.extensions, TenantTypes: options.tenantTypes})
	require.NoError(t, err)
	t.Cleanup(gw.Close)
	provider := gw.Provider()
	invalid := rest.Config{Host: "https://grafana.invalid", TLSClientConfig: rest.TLSClientConfig{CAData: []byte("not a certificate")}}
	_, first := provider.NewApp(app.Config{KubeConfig: invalid, ManifestData: *provider.Manifest().ManifestData})
	require.Error(t, first)
	_, second := provider.NewApp(app.Config{KubeConfig: rest.Config{Host: "https://grafana.example"}, ManifestData: *provider.Manifest().ManifestData})
	assert.Equal(t, first, second, "a later build does not retry the connection")
}

func mustJWKS(t *testing.T, host *platformhost.Host) string {
	t.Helper()
	keys, err := host.JWKS()
	require.NoError(t, err)
	return string(keys)
}
