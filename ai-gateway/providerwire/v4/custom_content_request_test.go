package v4

import (
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rawOptions(namespace, raw string) provider.ProviderOptions {
	return provider.ProviderOptions{namespace: provider.RawProviderOption{Key: namespace, Raw: []byte(raw)}}
}

func assertRawOptions(t *testing.T, namespace, want string, got provider.ProviderOptions) {
	t.Helper()
	require.Len(t, got, 1)
	raw, ok := got[namespace].(provider.RawProviderOption)
	require.True(t, ok)
	assert.JSONEq(t, want, string(raw.Raw))
}

func TestCustomContentRequest_MapsAssistantHistoryAndToolResults(t *testing.T) {
	const body = `{"prompt":[
		{"role":"assistant","content":[
			{"type":"text","text":"answer"},
			{"type":"custom","kind":"openai.compaction","providerOptions":{"openai":{"itemId":"cmp_1","encryptedContent":"ENC"}}},
			{"type":"custom","kind":"anthropic.fallback","providerOptions":{"anthropic":{"type":"fallback","from":{"model":"a"},"to":{"model":"b"}}}},
			{"type":"custom","kind":"example.kind"}
		]},
		{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"search","output":{"type":"content","value":[
			{"type":"text","text":"before"},
			{"type":"custom","providerOptions":{"anthropic":{"type":"tool-reference","toolName":"search"}}},
			{"type":"file","data":{"type":"data","data":"AA=="},"mediaType":"image/png"},
			{"type":"custom"}
		]}}]}
	]}`

	for _, streaming := range []string{"false", "true"} {
		t.Run("streaming="+streaming, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			request := validRequest(body)
			request.Header.Set(HeaderStreaming, streaming)

			response := harness.serve(request)

			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, 1, harness.model.callCount())
			prompt := harness.model.receivedOptions().Prompt
			require.Len(t, prompt, 2)

			history := prompt[0].Content
			require.Len(t, history, 4)
			assert.Equal(t, provider.ContentPartTypeText, history[0].Type)
			assert.Equal(t, provider.ContentPartTypeCustom, history[1].Type)
			assert.Equal(t, "openai.compaction", history[1].Kind)
			assert.Equal(t, rawOptions("openai", `{"itemId":"cmp_1","encryptedContent":"ENC"}`), history[1].ProviderOptions)
			assert.Equal(t, "anthropic.fallback", history[2].Kind)
			assert.Equal(t, "example.kind", history[3].Kind)
			assert.Nil(t, history[3].ProviderOptions)

			result := prompt[1].Content[0].Output
			require.NotNil(t, result)
			require.Len(t, result.Content, 4)
			assert.Equal(t, provider.ToolContentText, result.Content[0].Type)
			assert.Equal(t, provider.ToolContentCustom, result.Content[1].Type)
			assert.Equal(t, rawOptions("anthropic", `{"type":"tool-reference","toolName":"search"}`), result.Content[1].ProviderOptions)
			assert.Equal(t, provider.ToolContentFile, result.Content[2].Type)
			assert.Equal(t, provider.ToolContentCustom, result.Content[3].Type)
			assert.Nil(t, result.Content[3].ProviderOptions)
		})
	}
}

func TestCustomContentRequest_Refusals(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"custom in user message", `{"prompt":[{"role":"user","content":[{"type":"custom","kind":"p.x"}]}]}`, string(canonicalInvalidRequestError)},
		{"custom in tool message", `{"prompt":[{"role":"tool","content":[{"type":"custom","kind":"p.x"}]}]}`, string(canonicalInvalidRequestError)},
		{"undotted kind", `{"prompt":[{"role":"assistant","content":[{"type":"custom","kind":"unqualified"}]}]}`, string(canonicalInvalidRequestError)},
		{"missing kind", `{"prompt":[{"role":"assistant","content":[{"type":"custom"}]}]}`, string(canonicalInvalidRequestError)},
		{"non-object options", `{"prompt":[{"role":"assistant","content":[{"type":"custom","kind":"p.x","providerOptions":{"p":[]}}]}]}`, string(canonicalInvalidRequestError)},
		{"reserved namespace on custom part", `{"prompt":[{"role":"assistant","content":[{"type":"custom","kind":"p.x","providerOptions":{"gateway":{}}}]}]}`, string(reservedProviderOptionsError)},
		{"reserved namespace on tool-result custom entry", `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"custom","providerOptions":{"grafana":{}}}]}}]}]}`, string(reservedProviderOptionsError)},
		{"approval response options stay uninterpreted", `{"prompt":[{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"a","approved":false,"providerOptions":{"gateway":{}}}]}]}`, string(unsupportedToolApprovalsError)},
		{"nested options on text entry stay refused", `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"c","toolName":"t","output":{"type":"content","value":[{"type":"text","text":"x","providerOptions":{"p":{"a":1}}}]}}]}]}`, string(unsupportedProviderOptionsError)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())

			response := harness.serve(validRequest(tc.body))

			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.JSONEq(t, tc.want, response.Body.String())
			assert.Zero(t, harness.resolver.callCount())
			assert.Zero(t, harness.model.callCount())
		})
	}
}

func TestCustomContentRequest_RegisteredClientGolden(t *testing.T) {
	records := loadGolden(t, "custom-content.json")
	require.Len(t, records, 2)
	for index, record := range records {
		harness := newRuntimeHarness(t, testLimits())

		response := harness.serve(requestFromGolden(t, record))

		require.Equal(t, http.StatusOK, response.Code, "record %d: %s", index, response.Body.String())
		require.Equal(t, 1, harness.model.callCount())
		prompt := harness.model.receivedOptions().Prompt
		require.Len(t, prompt, 3)

		var kinds []string
		for _, part := range prompt[1].Content {
			if part.Type == provider.ContentPartTypeCustom {
				kinds = append(kinds, part.Kind)
			}
		}
		assert.Equal(t, []string{"openai.compaction", "anthropic.fallback", "example.kind"}, kinds)
		assertRawOptions(t, "openai", `{"itemId":"cmp_1","encryptedContent":"ENC"}`, prompt[1].Content[1].ProviderOptions)

		content := prompt[2].Content[0].Output.Content
		require.Len(t, content, 3)
		assert.Equal(t, provider.ToolContentCustom, content[1].Type)
		assertRawOptions(t, "anthropic", `{"type":"tool-reference","toolName":"search"}`, content[1].ProviderOptions)
		assert.Equal(t, provider.ToolContentCustom, content[2].Type)
		assert.Nil(t, content[2].ProviderOptions)
	}
}
