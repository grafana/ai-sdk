package v4

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderOptions_OpaqueValuesSurvive(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	opaque := `{"nested":{"nullValue":null,"falseValue":false,"zero":0,"empty":""},"array":[null,false,0,"",[],{}]}`
	body := `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi","providerOptions":{"part":` + opaque + `}}],"providerOptions":{"message":` + opaque + `}}],"providerOptions":{"call":` + opaque + `}}`
	response := harness.serve(validRequest(body))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	options := harness.model.receivedOptions()
	assert.Equal(t, opaque, string(options.ProviderOptions["call"].(provider.RawProviderOption).Raw))
	require.Len(t, options.Prompt, 1)
	assert.Equal(t, opaque, string(options.Prompt[0].ProviderOptions["message"].(provider.RawProviderOption).Raw))
	require.Len(t, options.Prompt[0].Content, 1)
	assert.Equal(t, opaque, string(options.Prompt[0].Content[0].ProviderOptions["part"].(provider.RawProviderOption).Raw))
}

func TestProviderOptions_MalformedNamespaceIsInvalidRequest(t *testing.T) {
	for name, raw := range map[string]string{"null": `null`, "array": `[]`, "scalar": `1`, "string": `"x"`} {
		t.Run(name, func(t *testing.T) {
			mapped, failure := mapWireProviderOptions(map[string]json.RawMessage{"ns": json.RawMessage(raw)})
			assert.Nil(t, mapped)
			require.NotNil(t, failure)
			assert.Empty(t, failure.safe.capability)
		})
	}
}

func TestProviderOptions_ReservedNamespaceIsRejected(t *testing.T) {
	for _, namespace := range []string{"grafana", "gateway", "grafana-ai-sdk"} {
		t.Run(namespace, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(`{"prompt":[],"providerOptions":{"` + namespace + `":{"tenant":"other"}}}`))
			require.Equal(t, http.StatusBadRequest, response.Code)
			assert.JSONEq(t, string(reservedProviderOptionsError), response.Body.String())
			assert.NotContains(t, response.Body.String(), "tenant")
			assert.Zero(t, harness.resolver.callCount())
			assert.Zero(t, harness.model.calls)
		})
	}
}

func TestCallHeaders_CaseInsensitiveDuplicateIsInvalid(t *testing.T) {
	mapped, failure := mapWireHeaders(map[string]string{"X-Foo": "a", "x-foo": "b"})
	assert.Nil(t, mapped)
	require.NotNil(t, failure)
	assert.Empty(t, failure.safe.capability)
}

func TestCallHeaders_MappedAndCasePreserved(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	response := harness.serve(validRequest(`{"prompt":[],"headers":{"X-Contract-Body":"value","AI-Language-Model-Id":"call"}}`))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, map[string]string{"X-Contract-Body": "value"}, harness.model.receivedOptions().Headers)
}

func TestProviderOptions_OrdinaryFieldsAreForwarded(t *testing.T) {
	for _, field := range []string{"model", "tools", "role", "type", "MCPServers", "Stream-Options", "content"} {
		t.Run(field, func(t *testing.T) {
			mapped, failure := mapWireProviderOptions(map[string]json.RawMessage{"ordinary": json.RawMessage(`{"` + field + `":{}}`)})
			require.Nil(t, failure)
			assert.Equal(t, json.RawMessage(`{"`+field+`":{}}`), mapped["ordinary"].(provider.RawProviderOption).Raw)
		})
	}
}

func TestProviderOptions_ReservedNamespaceIsCaseSignificant(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	response := harness.serve(validRequest(`{"prompt":[],"providerOptions":{"Grafana":{"kept":true}}}`))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, provider.ProviderOptions{"Grafana": provider.RawProviderOption{Key: "Grafana", Raw: json.RawMessage(`{"kept":true}`)}}, harness.model.receivedOptions().ProviderOptions)
}

func TestProviderOptions_ReasoningFileScopes(t *testing.T) {
	for _, tc := range []struct {
		name, options string
		status        int
		failure       []byte
	}{
		{name: "ordinary model", options: `{"anthropic":{"model":"ordinary"}}`, status: http.StatusOK},
		{name: "reserved", options: `{"grafana":{}}`, status: http.StatusBadRequest, failure: reservedProviderOptionsError},
		{name: "signature", options: `{"anthropic":{"signature":""}}`, status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(`{"prompt":[{"role":"assistant","content":[{"type":"reasoning-file","data":{"type":"data","data":""},"mediaType":"image/png","providerOptions":` + tc.options + `}]}]}`))
			require.Equal(t, tc.status, response.Code, response.Body.String())
			if tc.failure != nil {
				assert.JSONEq(t, string(tc.failure), response.Body.String())
				assert.Zero(t, harness.model.calls)
			}
		})
	}
}

func TestCallHeaders_ProtectedNamesAreRejected(t *testing.T) {
	for _, name := range []string{"Authorization", "proxy-authorization", "X-Access-Token", "x-grafana-id", "X-Api-Key", "api-key", "OpenAI-API-Key", "anthropic-api-key"} {
		t.Run(name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			body, err := json.Marshal(map[string]any{"prompt": []any{}, "headers": map[string]string{name: "caller-controlled"}})
			require.NoError(t, err)
			response := harness.serve(validRequest(string(body)))
			require.Equal(t, http.StatusBadRequest, response.Code)
			assert.JSONEq(t, string(protectedCallHeaderError), response.Body.String())
			assert.NotContains(t, response.Body.String(), "caller-controlled")
			assert.Zero(t, harness.model.calls)
		})
	}
}

func TestProtectedCallHeaders_CoverInboundAuthNames(t *testing.T) {
	for _, name := range []string{"authorization", "x-access-token", "x-grafana-id"} {
		_, protected := protectedCallHeaders[name]
		assert.True(t, protected)
	}
}
