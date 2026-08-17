package providerwirev4

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallOptions_ToolResultOutputVariants(t *testing.T) {
	cases := []struct {
		output provider.ToolResultOutput
		wire   string
	}{
		{output: provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}, wire: `{"type":"text","value":"ok"}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "boom"}, wire: `{"type":"error-text","value":"boom"}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"ok":true}`)}, wire: `{"type":"json","value":{"ok":true}}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`null`)}, wire: `{"type":"json","value":null}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"error":true}`)}, wire: `{"type":"error-json","value":{"error":true}}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`null`)}, wire: `{"type":"error-json","value":null}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: "value"}}}, wire: `{"type":"content","value":[{"type":"text","text":"value"}]}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: "denied"}, wire: `{"type":"execution-denied","reason":"denied"}`},
	}
	for _, tc := range cases {
		t.Run(string(tc.output.Type), func(t *testing.T) {
			options := provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", &tc.output))}}
			data, err := EncodeCallOptions(options)
			require.NoError(t, err)
			var encoded struct {
				Prompt []struct {
					Content []struct {
						Output json.RawMessage `json:"output"`
					} `json:"content"`
				} `json:"prompt"`
			}
			require.NoError(t, json.Unmarshal(data, &encoded))
			assert.JSONEq(t, tc.wire, string(encoded.Prompt[0].Content[0].Output))

			wire := `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":` + tc.wire + `}]}]}`
			decoded, err := decodeCallOptionsJSON([]byte(wire))
			require.NoError(t, err)
			assert.Equal(t, options, decoded)
		})
	}
}

func TestCallOptions_ToolResultOutputProviderOptionsEligibility(t *testing.T) {
	providerOptions := provider.ProviderOptions{
		"provider": provider.RawProviderOption{Key: "provider", Raw: json.RawMessage(`{"keep":true}`)},
	}
	eligible := []provider.ToolResultOutput{
		{Type: provider.ToolOutputText, Text: "ok", ProviderOptions: providerOptions},
		{Type: provider.ToolOutputErrorText, Text: "error", ProviderOptions: providerOptions},
		{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`null`), ProviderOptions: providerOptions},
		{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"error":true}`), ProviderOptions: providerOptions},
		{Type: provider.ToolOutputExecutionDenied, Reason: "denied", ProviderOptions: providerOptions},
	}
	for _, output := range eligible {
		t.Run(string(output.Type), func(t *testing.T) {
			options := provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", &output))}}
			encoded, err := EncodeCallOptions(options)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), `"providerOptions":{"provider":{"keep":true}}`)
			decoded, err := decodeCallOptionsJSON(encoded)
			require.NoError(t, err)
			assert.Equal(t, options, decoded)
		})
	}

	content := provider.ToolResultOutput{
		Type:            provider.ToolOutputContent,
		Content:         []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: "ok"}},
		ProviderOptions: providerOptions,
	}
	_, err := EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{
		provider.NewToolMessage(provider.ToolResultPart("call", "tool", &content)),
	}})
	require.Error(t, err)

	wire := `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"ok","providerOptions":{"provider":{"nested":true}}}],"providerOptions":{"provider":{"inactive":true}}}}]}]}`
	decoded, err := decodeCallOptionsJSON([]byte(wire))
	require.NoError(t, err)
	output := decoded.Prompt[0].Content[0].Output
	require.NotNil(t, output)
	assert.Empty(t, output.ProviderOptions)
	assert.Contains(t, output.Content[0].ProviderOptions, "provider")
	encoded, err := EncodeCallOptions(decoded)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"ok","providerOptions":{"provider":{"nested":true}}}]}}]}]}`, string(encoded))
}
