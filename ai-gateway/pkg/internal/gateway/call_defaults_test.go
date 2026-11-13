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

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/calldefaults"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const (
	defaultInstructionsMember = `"defaultInstructions":{"messages":[{"content":"Be brief."},{"content":"Cite sources."}]}`
	defaultSettingsMember     = `"defaultSettings":{"temperature":0.25,"maxOutputTokens":48,"stopSequences":["END"],"seed":9007199254740991}`
	promptOnlyBody            = `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`
	clientSystemBody          = `{"prompt":[{"role":"system","content":"Client instructions."},{"role":"user","content":[{"type":"text","text":"hi"}]}],"maxOutputTokens":16,"temperature":0}`
)

func shippedOptions() harnessOptions {
	return harnessOptions{tenantTypes: tenanttype.All(), extensions: calldefaults.Definitions()}
}

// anthropicRequest is the part of a Messages request the call defaults
// affect.
type anthropicRequest struct {
	System []struct {
		Text string `json:"text"`
	} `json:"system"`
	Temperature   *float64 `json:"temperature"`
	MaxTokens     int      `json:"max_tokens"`
	StopSequences []string `json:"stop_sequences"`
}

func (h *harness) lastProviderRequest(t *testing.T) anthropicRequest {
	t.Helper()
	h.anthropic.mu.Lock()
	body := h.anthropic.bodies[len(h.anthropic.bodies)-1]
	h.anthropic.mu.Unlock()
	var request anthropicRequest
	require.NoError(t, json.Unmarshal([]byte(body), &request), body)
	return request
}

func systemTexts(request anthropicRequest) []string {
	var texts []string
	for _, block := range request.System {
		texts = append(texts, block.Text)
	}
	return texts
}

func TestCallDefaults_ApplyEachNamespacesResources(t *testing.T) {
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, shippedOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	create := func(namespace, document string) {
		t.Helper()
		code, body := sendObject(t, ctx, http.MethodPost, namespacedURL(h.server.URL, namespace, "middlewares"), h.namespaceToken(t, namespace), document)
		require.Equal(t, http.StatusCreated, code, body)
	}
	create("stacks-1", middlewareDocument("default-instructions", "default-instructions", true, defaultInstructionsMember))
	create("stacks-1", middlewareDocument("default-settings", "default-settings", true, defaultSettingsMember))
	create("stacks-2", middlewareDocument("default-instructions", "default-instructions", true, defaultInstructionsMember))
	create("stacks-2", middlewareDocument("default-settings", "default-settings", true, defaultSettingsMember))
	code, body := sendObject(t, ctx, http.MethodPut, namespacedURL(h.server.URL, "stacks-2", "middlewares/default-instructions"), h.namespaceToken(t, "stacks-2"), middlewareDocument("default-instructions", "default-instructions", false, defaultInstructionsMember))
	require.Equal(t, http.StatusOK, code, body)
	code, body = sendObject(t, ctx, http.MethodDelete, namespacedURL(h.server.URL, "stacks-2", "middlewares/default-settings"), h.namespaceToken(t, "stacks-2"), "")
	require.Equal(t, http.StatusOK, code, body)

	call := func(namespace, body string, streaming bool) anthropicRequest {
		t.Helper()
		code, response := do(t, languageRequest(t, ctx, namespacedURL(h.server.URL, namespace, "language-model"), body, streaming, accessToken(h.namespaceToken(t, namespace))))
		require.Equal(t, http.StatusOK, code, response)
		return h.lastProviderRequest(t)
	}

	for _, streaming := range []bool{false, true} {
		applied := call("stacks-1", promptOnlyBody, streaming)
		assert.Equal(t, []string{"Be brief.", "Cite sources."}, systemTexts(applied), "streaming %t", streaming)
		assert.Equal(t, new(0.25), applied.Temperature)
		assert.Equal(t, 48, applied.MaxTokens)
		assert.Equal(t, []string{"END"}, applied.StopSequences)
	}

	own := call("stacks-1", clientSystemBody, false)
	assert.Equal(t, []string{"Client instructions."}, systemTexts(own), "a client system message suppresses the default instructions")
	assert.Equal(t, new(0.0), own.Temperature, "the call's own settings win")
	assert.Equal(t, 16, own.MaxTokens)
	assert.Equal(t, []string{"END"}, own.StopSequences, "settings the call leaves unset still apply")

	for _, namespace := range []string{"stacks-2", "stacks-3"} {
		unchanged := call(namespace, promptOnlyBody, false)
		assert.Empty(t, unchanged.System, namespace)
		assert.Nil(t, unchanged.Temperature, namespace)
		assert.NotEqual(t, 48, unchanged.MaxTokens, namespace)
		assert.Empty(t, unchanged.StopSequences, namespace)
	}
	assert.Contains(t, h.metrics(t), `grafana_ai_gateway_tenant_config_reads_total{result="success"} 3`, "one read per namespace")
}

func TestMiddlewareAdmission_ShippedTypes(t *testing.T) {
	admissionLogs := captureAdmissionLogs(t)
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, shippedOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token := h.serviceToken(t, testInternalService)
	instructions := func(member string) string {
		return middlewareDocument("default-instructions", "default-instructions", true, `"defaultInstructions":`+member)
	}
	settings := func(member string) string {
		return middlewareDocument("default-settings", "default-settings", true, `"defaultSettings":`+member)
	}
	quoted := `"` + middlewareSentinel + `"`

	for _, tc := range []struct {
		name     string
		document string
		reason   string
	}{
		{name: "settings headers", document: settings(`{"headers":{"x-tenant":` + quoted + `}}`), reason: "spec.defaultSettings does not match the extension's schema at /"},
		{name: "settings provider options", document: settings(`{"providerOptions":{"anthropic":{"note":` + quoted + `}}}`), reason: "spec.defaultSettings does not match the extension's schema at /"},
		{name: "settings tools", document: settings(`{"tools":[{"type":"function","name":` + quoted + `}]}`), reason: "spec.defaultSettings does not match the extension's schema at /"},
		{name: "settings tool choice", document: settings(`{"toolChoice":{"type":"tool","toolName":` + quoted + `}}`), reason: "spec.defaultSettings does not match the extension's schema at /"},
		{name: "settings response format", document: settings(`{"responseFormat":{"type":"json","name":` + quoted + `}}`), reason: "spec.defaultSettings does not match the extension's schema at /"},
		{name: "settings reasoning", document: settings(`{"reasoning":` + quoted + `}`), reason: "spec.defaultSettings does not match the extension's schema at /"},
		{name: "settings undeclared key", document: settings(`{"temperature":0.2,"` + middlewareSentinel + `":1}`), reason: "spec.defaultSettings does not match the extension's schema at /"},
		{name: "settings empty", document: settings(`{}`), reason: "spec.defaultSettings does not match the extension's schema at /"},
		{name: "settings wrong type", document: settings(`{"temperature":` + quoted + `}`), reason: "spec.defaultSettings does not match the extension's schema at /temperature"},
		{name: "settings zero output tokens", document: settings(`{"maxOutputTokens":0,"stopSequences":[` + quoted + `]}`), reason: "spec.defaultSettings does not match the extension's schema at /maxOutputTokens"},
		{name: "settings fractional top k", document: settings(`{"topK":1.5}`), reason: "spec.defaultSettings does not match the extension's schema at /topK"},
		{name: "settings seed above the safe range", document: settings(`{"seed":9007199254740992}`), reason: "spec.defaultSettings does not match the extension's schema at /seed"},
		{name: "settings seed below the safe range", document: settings(`{"seed":-9007199254740992}`), reason: "spec.defaultSettings does not match the extension's schema at /seed"},
		{name: "settings stop sequence type", document: settings(`{"stopSequences":[1]}`), reason: "spec.defaultSettings does not match the extension's schema at /stopSequences/0"},
		{name: "instructions empty list", document: instructions(`{"messages":[]}`), reason: "spec.defaultInstructions does not match the extension's schema at /messages"},
		{name: "instructions empty content", document: instructions(`{"messages":[{"content":` + quoted + `},{"content":""}]}`), reason: "spec.defaultInstructions does not match the extension's schema at /messages/1/content"},
		{name: "instructions missing content", document: instructions(`{"messages":[{}]}`), reason: "spec.defaultInstructions does not match the extension's schema at /messages/0"},
		{name: "instructions message provider options", document: instructions(`{"messages":[{"content":"a","providerOptions":{"anthropic":{"note":` + quoted + `}}}]}`), reason: "spec.defaultInstructions does not match the extension's schema at /messages/0"},
		{name: "instructions message role", document: instructions(`{"messages":[{"content":"a","role":` + quoted + `}]}`), reason: "spec.defaultInstructions does not match the extension's schema at /messages/0"},
		{name: "instructions provider options", document: instructions(`{"messages":[{"content":"a"}],"providerOptions":{"anthropic":{"note":` + quoted + `}}}`), reason: "spec.defaultInstructions does not match the extension's schema at /"},
		{name: "instructions content type", document: instructions(`{"messages":[{"content":["` + middlewareSentinel + `"]}]}`), reason: "spec.defaultInstructions does not match the extension's schema at /messages/0/content"},
		{name: "enabled without a member", document: middlewareDocument("default-settings", "default-settings", true, ""), reason: "spec.defaultSettings is required when spec.enabled is true"},
		{name: "unregistered type", document: middlewareDocument("model-access", "model-access", true, `"modelAccess":{}`), reason: `spec.type \"model-access\" is not a tenant extension of this Gateway`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), token, tc.document)
			assert.Equal(t, http.StatusBadRequest, code, body)
			assert.Contains(t, body, "admission hook for Middleware denied the request: middleware: "+tc.reason)
			assert.NotContains(t, body, middlewareSentinel)
		})
	}

	for _, tc := range []struct {
		name     string
		document string
	}{
		{name: "instructions example", document: instructions(string(tenanttype.DefaultInstructions().Example))},
		{name: "settings example", document: settings(string(tenanttype.DefaultSettings().Example))},
		{name: "settings bounds", document: settings(`{"maxOutputTokens":1,"seed":9007199254740991,"topK":-9007199254740991,"stopSequences":[]}`)},
		{name: "settings integers with a fraction or an exponent", document: settings(`{"maxOutputTokens":1.0,"topK":4e1}`)},
		{name: "settings disabled without a member", document: middlewareDocument("default-settings", "default-settings", false, "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), token, tc.document)
			require.Equal(t, http.StatusCreated, code, body)
			name := "default-settings"
			if strings.Contains(tc.document, "default-instructions") {
				name = "default-instructions"
			}
			code, body = sendObject(t, ctx, http.MethodDelete, middlewareURL(h.server.URL, name), token, "")
			require.Equal(t, http.StatusOK, code, body)
		})
	}
	assert.NotContains(t, admissionLogs.String(), middlewareSentinel)
	assert.NotContains(t, h.logs.String(), middlewareSentinel)
}

func TestTenantConfiguration_ShippedDefinitionsReadEachNamespace(t *testing.T) {
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, shippedOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for range 2 {
		code, body := do(t, languageRequest(t, ctx, h.url("language-model"), promptOnlyBody, false, accessToken(h.serviceToken(t, testInternalService))))
		require.Equal(t, http.StatusOK, code, body)
		request := h.lastProviderRequest(t)
		assert.Empty(t, request.System)
		assert.Nil(t, request.Temperature)
	}
	assert.Contains(t, h.metrics(t), `grafana_ai_gateway_tenant_config_reads_total{result="success"} 1`, "one read, cached for the TTL")
	assert.NotEmpty(t, h.exchanges.recorded(), "reads go through the plugin's token exchange")
}

func TestTenantConfiguration_ShippedDefinitionsWithoutAPIAccess(t *testing.T) {
	options := shippedOptions()
	options.noAPIAccess = true
	h := newHarnessWithOptions(t, platformhost.TopologyCloud, options)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token := h.serviceToken(t, testInternalService)
	code, body := do(t, languageRequest(t, ctx, h.url("language-model"), promptOnlyBody, false, accessToken(token)))
	require.Equal(t, http.StatusOK, code, body)
	assert.Empty(t, h.lastProviderRequest(t).System)
	assert.Empty(t, h.exchanges.recorded())
	assert.Equal(t, 1, strings.Count(h.logs.String(), "gateway tenant configuration is not applied"))
	code, body = sendObject(t, ctx, http.MethodPost, middlewareURL(h.server.URL, ""), token, middlewareDocument("default-settings", "default-settings", true, defaultSettingsMember))
	assert.Equal(t, http.StatusBadRequest, code, body)
}
