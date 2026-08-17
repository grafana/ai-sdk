package providerwirev4

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallOptions_GatewayNamespaceIsRemovedOrRejected(t *testing.T) {
	for _, wire := range []string{
		`{"prompt":[]}`,
		`{"prompt":[],"providerOptions":{"gateway":{}}}`,
		`{"prompt":[],"providerOptions":{"provider":{"keep":true},"gateway":{}}}`,
	} {
		decoded, err := decodeCallOptionsJSON([]byte(wire))
		require.NoError(t, err)
		assert.NotContains(t, decoded.ProviderOptions, "gateway")
	}

	for _, gateway := range []string{
		`null`,
		`[]`,
		`{"models":["fallback"]}`,
		`{"only":["provider"]}`,
		`{"order":["provider"]}`,
		`{"byok":{"provider":[{"credentialId":"reference"}]}}`,
		`{"serviceTier":"priority"}`,
		`{"future":null}`,
	} {
		_, err := decodeCallOptionsJSON([]byte(`{"prompt":[],"providerOptions":{"gateway":` + gateway + `}}`))
		require.Error(t, err)
	}

	empty := provider.CallOptions{ProviderOptions: provider.ProviderOptions{
		"gateway":  provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(`{}`)},
		"provider": provider.RawProviderOption{Key: "provider", Raw: json.RawMessage(`{"keep":true}`)},
	}}
	encoded, err := EncodeCallOptions(empty)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"gateway"`)

	nonEmpty := empty
	nonEmpty.ProviderOptions = provider.ProviderOptions{"gateway": provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(`{"models":["fallback"]}`)}}
	_, err = EncodeCallOptions(nonEmpty)
	require.Error(t, err)
}

func TestCallOptions_NestedGatewayProviderOptionsAreRejected(t *testing.T) {
	gatewayOptions := provider.ProviderOptions{"gateway": provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(`{"models":["fallback"]}`)}}
	validOutput := provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}
	encodeCases := []struct {
		name    string
		options provider.CallOptions
	}{
		{name: "message", options: provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleUser, Content: []provider.ContentPart{}, ProviderOptions: gatewayOptions}}}},
		{name: "content part", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeText, Text: "text", ProviderOptions: gatewayOptions})}}},
		{name: "function tool", options: provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool", InputSchema: json.RawMessage(`{}`), ProviderOptions: gatewayOptions}}}},
		{name: "tool output", options: provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ContentPart{Type: provider.ContentPartTypeToolResult, ToolCallID: "call", ToolName: "tool", Output: &provider.ToolResultOutput{Type: validOutput.Type, Text: validOutput.Text, ProviderOptions: gatewayOptions}})}}},
		{name: "tool result content", options: provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: "text", ProviderOptions: gatewayOptions}}}))}}},
	}
	for _, tc := range encodeCases {
		t.Run("encode-"+tc.name, func(t *testing.T) {
			_, err := EncodeCallOptions(tc.options)
			require.Error(t, err)
		})
	}

	decodeCases := []struct {
		name string
		wire string
	}{
		{name: "message", wire: `{"prompt":[{"role":"user","content":[],"providerOptions":{"gateway":{}}}]}`},
		{name: "content part", wire: `{"prompt":[{"role":"user","content":[{"type":"text","text":"text","providerOptions":{"gateway":{}}}]}]}`},
		{name: "function tool", wire: `{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":{},"providerOptions":{"gateway":{}}}]}`},
		{name: "provider tool inactive options", wire: `{"prompt":[],"tools":[{"type":"provider","id":"p.tool","name":"tool","args":{},"providerOptions":{"gateway":{}}}]}`},
		{name: "tool output", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"text","value":"ok","providerOptions":{"gateway":{}}}}]}]}`},
		{name: "inactive content tool output gateway", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[],"providerOptions":{"gateway":{}}}}]}]}`},
		{name: "tool result content", wire: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"text","providerOptions":{"gateway":{}}]}}]}]}`},
	}
	for _, tc := range decodeCases {
		t.Run("decode-"+tc.name, func(t *testing.T) {
			_, err := decodeCallOptionsJSON([]byte(tc.wire))
			require.Error(t, err)
		})
	}

	topLevel := provider.CallOptions{ProviderOptions: provider.ProviderOptions{
		"gateway": provider.RawProviderOption{Key: "gateway", Raw: json.RawMessage(`{"models":["fallback"]}`)},
	}}
	_, err := EncodeCallOptions(topLevel)
	require.Error(t, err)

	nestedUnknown := `{"prompt":[{"role":"user","content":[{"type":"text","text":"text","providerOptions":{"provider":{"part":true}}}],"providerOptions":{"provider":{"message":true}}},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"ok","providerOptions":{"provider":{"content":true}}}],"providerOptions":{"provider":{"inactive":true}}}}]}],"tools":[{"type":"function","name":"tool","inputSchema":{},"providerOptions":{"provider":{"tool":true}}}]}`
	decoded, err := decodeCallOptionsJSON([]byte(nestedUnknown))
	require.NoError(t, err)
	encoded, err := EncodeCallOptions(decoded)
	require.NoError(t, err)
	expected := `{"prompt":[{"role":"user","content":[{"type":"text","text":"text","providerOptions":{"provider":{"part":true}}}],"providerOptions":{"provider":{"message":true}}},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"ok","providerOptions":{"provider":{"content":true}}}]}}]}],"tools":[{"type":"function","name":"tool","inputSchema":{},"providerOptions":{"provider":{"tool":true}}}]}`
	assert.JSONEq(t, expected, string(encoded))
}
