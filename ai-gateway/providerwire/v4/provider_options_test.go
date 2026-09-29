package v4

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
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
	assert.Equal(t, opaque, string(options.ProviderOptions["call"].(provider.RawProviderOption).Raw), "namespace bytes survive unchanged")
	require.Len(t, options.Prompt, 1)
	assert.Equal(t, opaque, string(options.Prompt[0].ProviderOptions["message"].(provider.RawProviderOption).Raw))
	require.Len(t, options.Prompt[0].Content, 1)
	assert.Equal(t, opaque, string(options.Prompt[0].Content[0].ProviderOptions["part"].(provider.RawProviderOption).Raw))
}

func TestProviderOptions_MalformedNamespaceIsInvalidRequest(t *testing.T) {
	// Each of these is schema-rejected before the mapper, so assert the mapper
	// itself refuses them: it is the last guard if the schema ever loosens.
	for name, raw := range map[string]string{
		"null":   `null`,
		"array":  `[]`,
		"scalar": `1`,
		"string": `"x"`,
	} {
		t.Run(name, func(t *testing.T) {
			mapped, failure := mapWireProviderOptions(map[string]json.RawMessage{"ns": json.RawMessage(raw)})
			assert.Nil(t, mapped)
			require.NotNil(t, failure)
			assert.Empty(t, failure.safe.capability, "a malformed namespace is an invalid request, not a capability refusal")
		})
	}
}

func TestProviderOptions_ReservedNamespaceIsRejected(t *testing.T) {
	for _, namespace := range []string{"grafana", "gateway", "grafana-ai-sdk"} {
		t.Run(namespace, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			body := `{"prompt":[],"providerOptions":{"` + namespace + `":{"tenant":"other"}}}`
			response := harness.serve(validRequest(body))
			require.Equal(t, http.StatusBadRequest, response.Code)
			assert.JSONEq(t, string(reservedProviderOptionsError), response.Body.String())
			assert.NotContains(t, response.Body.String(), "tenant", "the refusal never echoes caller input")
			assert.Zero(t, harness.model.calls, "the reserved namespace is refused before the model runs")
		})
	}
}

func TestCallHeaders_CaseInsensitiveDuplicateIsInvalid(t *testing.T) {
	mapped, failure := mapWireHeaders(map[string]string{"X-Foo": "a", "x-foo": "b"})
	assert.Nil(t, mapped)
	require.NotNil(t, failure)
	assert.Empty(t, failure.safe.capability, "a duplicated header is an invalid request, not a policy refusal")
}

func TestCallHeaders_MappedAndCasePreserved(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	// The HTTP contract round-trips a protocol name in the body, so the request
	// is accepted, but that name addresses this runtime and not a backend.
	body := `{"prompt":[],"headers":{"X-Contract-Body":"value","AI-Language-Model-Id":"call"}}`

	response := harness.serve(validRequest(body))

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, map[string]string{"X-Contract-Body": "value"},
		harness.model.receivedOptions().Headers,
		"ordinary keys keep the caller's spelling and protocol names are not forwarded")
}

func TestProviderOptions_ProtectedFieldIsRejectedAtEveryLevel(t *testing.T) {
	// Every level maps through mapWireProviderOptions, so each call site is
	// covered: a per-level shortcut would otherwise regress two of them silently.
	for name, body := range map[string]string{
		"call level":       `{"prompt":[],"providerOptions":{"ns":{"model":"someone-elses-model"}}}`,
		"message level":    `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi"}],"providerOptions":{"ns":{"messages":[]}}}]}`,
		"system level":     `{"prompt":[{"role":"system","content":"hi","providerOptions":{"ns":{"tools":[]}}}]}`,
		"part level":       `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi","providerOptions":{"ns":{"response_format":{"type":"json_schema"}}}}]}]}`,
		"part type":        `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi","providerOptions":{"openaiCompatible":{"type":"image_url","image_url":{"url":"https://caller.example/x.png"}}}}]}]}`,
		"message role":     `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi"}],"providerOptions":{"openaiCompatible":{"role":"tool","tool_call_id":"call_1"}}}]}`,
		"message content":  `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi"}],"providerOptions":{"openaiCompatible":{"content":[{"type":"image_url","image_url":{"url":"https://caller.example/x.png"}}]}}}]}`,
		"file part":        `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"data","data":""},"mediaType":"image/png","providerOptions":{"ns":{"model":"other"}}}]}]}`,
		"function tool":    `{"prompt":[],"tools":[{"type":"function","name":"search","inputSchema":{},"providerOptions":{"ns":{"model":"other"}}}]}`,
		"tool-result file": `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"file","data":{"type":"data","data":""},"mediaType":"image/png","providerOptions":{"ns":{"model":"other"}}}]}}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())

			response := harness.serve(validRequest(body))

			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			assert.JSONEq(t, string(protectedProviderOptionError), response.Body.String())
			assert.Zero(t, harness.resolver.callCount(), "refused before the model is resolved")
			assert.Zero(t, harness.model.calls, "refused before the model runs")
		})
	}
}

func TestProviderOptions_ProtectedFieldsCoverTheRefusedCapabilities(t *testing.T) {
	// A capability the wire refuses must not be reachable by restating it as a
	// provider option, because providers merge unknown option fields into the
	// request body.
	for _, field := range []string{"tools", "toolChoice", "tool_choice", "functions", "function_call", "responseFormat", "response_format", "stream", "streamOptions", "stream_options"} {
		_, protected := protectedProviderOptionFields[normalizeProviderOptionField(field)]
		assert.True(t, protected, "%s restates a decision this runtime makes at the wire level", field)
	}
}

func TestProviderOptions_ProtectedFieldSpellingsAreRejected(t *testing.T) {
	// providers/anthropic decodes options with encoding/json, which matches field
	// names case-insensitively, so every spelling it would read must be refused.
	for _, field := range []string{"MCPServers", "mcpservers", "MCP_SERVERS", "Container", "Model", "Fallbacks", "function_call", "Stream-Options", "Content", "content"} {
		t.Run(field, func(t *testing.T) {
			mapped, failure := mapWireProviderOptions(map[string]json.RawMessage{"anthropic": json.RawMessage(`{"` + field + `":[]}`)})
			assert.Nil(t, mapped)
			require.NotNil(t, failure)
			assert.Equal(t, capabilityProtectedProviderOption, failure.safe.capability)
		})
	}
}

func TestProviderOptions_OrdinaryFieldsAreForwarded(t *testing.T) {
	// Normalization must not widen the set: fields that merely contain a
	// protected word still reach the backend.
	for _, field := range []string{"thinking", "effort", "betas", "modelVersion", "toolStreaming", "disableParallelToolUse", "user"} {
		t.Run(field, func(t *testing.T) {
			mapped, failure := mapWireProviderOptions(map[string]json.RawMessage{"anthropic": json.RawMessage(`{"` + field + `":{}}`)})
			require.Nil(t, failure)
			assert.Contains(t, mapped, "anthropic")
		})
	}
}

func TestProviderOptions_ReservedNamespaceIsCaseSignificant(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	body := `{"prompt":[],"providerOptions":{"Grafana":{"kept":true}}}`

	response := harness.serve(validRequest(body))

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, provider.ProviderOptions{
		"Grafana": provider.RawProviderOption{Key: "Grafana", Raw: json.RawMessage(`{"kept":true}`)},
	}, harness.model.receivedOptions().ProviderOptions, "namespaces are compared exactly")
}

func TestProviderOptions_UnsupportedPartReportsItsOwnFamily(t *testing.T) {
	for name, body := range map[string]string{
		"reserved options": `{"prompt":[{"role":"assistant","content":[{"type":"reasoning-file","data":{"type":"data","data":""},"mediaType":"image/png","providerOptions":{"grafana":{}}}]}]}`,
		"ordinary options": `{"prompt":[{"role":"assistant","content":[{"type":"reasoning-file","data":{"type":"data","data":""},"mediaType":"image/png","providerOptions":{"ns":{}}}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(body))
			require.Equal(t, http.StatusBadRequest, response.Code)
			assert.JSONEq(t, string(unsupportedReasoningContentError), response.Body.String())
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
			assert.Zero(t, harness.model.calls, "a protected header never reaches a backend")
		})
	}
}

// The inbound edge strips these from outer headers because a trusted proxy owns
// them. Body-carried headers must refuse the same names, or the body becomes a
// way around that check.
func TestProtectedCallHeaders_CoverInboundAuthNames(t *testing.T) {
	for _, name := range []string{"authorization", "x-access-token", "x-grafana-id"} {
		_, protected := protectedCallHeaders[name]
		assert.True(t, protected, "%s must stay protected for body-carried headers", name)
	}
}

func TestProviderOptionPolicy_ForwardsOnlyTheSelectedBackendsNamespaces(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.resolver.resolved.ProviderOptions = catalog.ProviderOptionPolicy{Namespaces: []string{"ollama", "openaiCompatible"}}
	body := `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi","providerOptions":{"openaiCompatible":{"kept":1},"anthropic":{"cacheControl":{"type":"ephemeral"}}}}],"providerOptions":{"anthropic":{"x":1}}}],` +
		`"providerOptions":{"ollama":{"num_ctx":4096},"anthropic":{"thinking":{"type":"enabled"}},"openai":{"user":"u"}}}`

	response := harness.serve(validRequest(body))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	options := harness.model.receivedOptions()
	assert.Equal(t, provider.ProviderOptions{
		"ollama": provider.RawProviderOption{Key: "ollama", Raw: json.RawMessage(`{"num_ctx":4096}`)},
	}, options.ProviderOptions, "call level keeps only the selected backend's namespace, bytes intact")
	require.Len(t, options.Prompt, 1)
	assert.Nil(t, options.Prompt[0].ProviderOptions, "message level drops every other backend's namespace")
	assert.Equal(t, provider.ProviderOptions{
		"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"kept":1}`)},
	}, options.Prompt[0].Content[0].ProviderOptions, "part level applies the same rule")
}

func TestProviderOptionPolicy_FunctionAndNestedFileOptions(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.resolver.resolved.ProviderOptions = catalog.ProviderOptionPolicy{
		Namespaces: []string{"anthropic"},
		Fields:     map[string][]string{"anthropic": {"cacheControl"}},
	}
	options := `{"anthropic":{"cacheControl":{"type":"ephemeral"},"other":"private"},"openai":{"user":"private"}}`
	body := `{"tools":[{"type":"function","name":"search","inputSchema":{},"providerOptions":` + options + `}],` +
		`"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"search","output":` +
		`{"type":"content","value":[{"type":"file","data":{"type":"data","data":""},"mediaType":"application/pdf","providerOptions":` + options + `}]}}]}]}`
	response := harness.serve(validRequest(body))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	received := harness.model.receivedOptions()
	require.Len(t, received.Tools, 1)
	require.Len(t, received.Prompt, 1)
	require.Len(t, received.Prompt[0].Content, 1)
	require.NotNil(t, received.Prompt[0].Content[0].Output)
	require.Len(t, received.Prompt[0].Content[0].Output.Content, 1)
	for _, scoped := range []provider.ProviderOptions{
		received.Tools[0].ProviderOptions,
		received.Prompt[0].Content[0].Output.Content[0].ProviderOptions,
	} {
		require.Len(t, scoped, 1)
		value, ok := scoped["anthropic"].(provider.RawProviderOption)
		require.True(t, ok)
		assert.JSONEq(t, `{"cacheControl":{"type":"ephemeral"}}`, string(value.Raw))
	}
}

func TestProviderOptionPolicy_ZeroValueForwardsNothing(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.resolver.resolved.ProviderOptions = catalog.ProviderOptionPolicy{}

	response := harness.serve(validRequest(`{"prompt":[],"providerOptions":{"anthropic":{"thinking":{"type":"enabled"}}}}`))

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Nil(t, harness.model.receivedOptions().ProviderOptions, "an unclassified backend receives no caller options")
}

func TestProviderOptionPolicy_FieldAllowlistRemovesOtherFields(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	harness.resolver.resolved.ProviderOptions = catalog.ProviderOptionPolicy{
		Namespaces: []string{"anthropic"},
		Fields:     map[string][]string{"anthropic": {"thinking", "cacheControl"}},
	}
	body := `{"prompt":[{"role":"user","content":[{"type":"text","text":"hi","providerOptions":{"anthropic":{"cache_control":{"type":"ephemeral"}}}}]}],` +
		`"providerOptions":{"anthropic":{"thinking":{"type":"enabled","budgetTokens":1024},"futureServerFeature":{"url":"https://caller.example"}}}}`

	response := harness.serve(validRequest(body))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	options := harness.model.receivedOptions()
	assert.JSONEq(t, `{"thinking":{"type":"enabled","budgetTokens":1024}}`,
		string(options.ProviderOptions["anthropic"].(provider.RawProviderOption).Raw),
		"a field the backend is not known to read is removed, and kept values are intact")
	assert.Equal(t, `{"cache_control":{"type":"ephemeral"}}`,
		string(options.Prompt[0].Content[0].ProviderOptions["anthropic"].(provider.RawProviderOption).Raw),
		"an allowed field under another spelling is kept, and an untouched namespace keeps its bytes")
}
