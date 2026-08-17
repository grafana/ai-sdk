package providerwirev4

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallOptions_EmptyInlineTextFileDataRoundTrips(t *testing.T) {
	wires := []string{
		`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":""},"mediaType":"text/plain"}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"file","data":{"type":"text","text":""},"mediaType":"text/plain"}]}}]}]}`,
	}
	for _, wire := range wires {
		decoded, err := decodeCallOptionsJSON([]byte(wire))
		require.NoError(t, err)
		encoded, err := EncodeCallOptions(decoded)
		require.NoError(t, err)
		assert.JSONEq(t, wire, string(encoded))
	}
}

func TestProviderReferenceValidationInRequestAndToolResultFiles(t *testing.T) {
	invalid := []json.RawMessage{
		json.RawMessage(`null`),
		json.RawMessage(`[]`),
		json.RawMessage(`{"p":null}`),
		json.RawMessage(`{"p":1}`),
		json.RawMessage(`{"type":"file-id"}`),
	}
	for i, reference := range invalid {
		t.Run(fmt.Sprintf("request-encode-%d", i), func(t *testing.T) {
			options := provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("application/pdf", provider.DataContent{Reference: reference}))}}
			_, err := EncodeCallOptions(options)
			require.Error(t, err)
		})
		t.Run(fmt.Sprintf("tool-result-encode-%d", i), func(t *testing.T) {
			output := &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentFile, Data: &provider.DataContent{Reference: reference}, MediaType: "application/pdf"}}}
			options := provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", output))}}
			_, err := EncodeCallOptions(options)
			require.Error(t, err)
		})
	}

	invalidWires := []string{
		`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"p":null}},"mediaType":"application/pdf"}]}]}`,
		`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"type":"file-id"}},"mediaType":"application/pdf"}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"file","data":{"type":"reference","reference":{"type":"file-id"}},"mediaType":"application/pdf"}]}}]}]}`,
	}
	for _, wire := range invalidWires {
		_, err := decodeCallOptionsJSON([]byte(wire))
		require.Error(t, err)
	}

	validWires := []string{
		`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"provider":"file-id"}},"mediaType":"application/pdf"}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"file","data":{"type":"reference","reference":{"provider":"file-id"}},"mediaType":"application/pdf"}]}}]}]}`,
	}
	for _, wire := range validWires {
		decoded, err := decodeCallOptionsJSON([]byte(wire))
		require.NoError(t, err)
		encoded, err := EncodeCallOptions(decoded)
		require.NoError(t, err)
		assert.JSONEq(t, wire, string(encoded))
	}
}
