package providerwirev4

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallOptions_PinnedContentAndFileDataGoldens(t *testing.T) {
	cases := []struct {
		name    string
		options provider.CallOptions
		wire    string
	}{
		{name: "text", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.TextPart("hello"))}}, wire: `{"prompt":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`},
		{name: "reasoning", options: provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ReasoningPart("thinking"))}}, wire: `{"prompt":[{"role":"assistant","content":[{"type":"reasoning","text":"thinking"}]}]}`},
		{name: "custom", options: provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.CustomPart("provider.custom"))}}, wire: `{"prompt":[{"role":"assistant","content":[{"type":"custom","kind":"provider.custom"}]}]}`},
		{name: "file data", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", provider.DataContent{Base64: "AAEC"}))}}, wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"data","data":"AAEC"},"mediaType":"image/png"}]}]}`},
		{name: "file URL", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", provider.DataContent{URL: "https://example.com/file.png"}))}}, wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"url","url":"https://example.com/file.png"},"mediaType":"image/png"}]}]}`},
		{name: "file reference", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("application/pdf", provider.DataContent{Reference: json.RawMessage(`{"provider":"file-1"}`)}))}}, wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"provider":"file-1"}},"mediaType":"application/pdf"}]}]}`},
		{name: "file text", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("text/plain", provider.DataContent{Text: "inline"}))}}, wire: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":"inline"},"mediaType":"text/plain"}]}]}`},
		{name: "reasoning file", options: provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ReasoningFilePart("image/png", provider.DataContent{URL: "https://example.com/reasoning.png"}))}}, wire: `{"prompt":[{"role":"assistant","content":[{"type":"reasoning-file","data":{"type":"url","url":"https://example.com/reasoning.png"},"mediaType":"image/png"}]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := EncodeCallOptions(tc.options)
			require.NoError(t, err)
			assert.JSONEq(t, tc.wire, string(encoded))

			decoded, err := decodeCallOptionsJSON([]byte(tc.wire))
			require.NoError(t, err)
			assert.Equal(t, tc.options, decoded)
		})
	}
}

func TestCallOptions_RoleContentMatrix(t *testing.T) {
	approved := false
	textOutput := &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}
	contentOutput := &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{
		{Type: provider.ToolContentText, Text: "text"},
		{Type: provider.ToolContentFile, Data: &provider.DataContent{URL: "https://example.com/file"}, MediaType: "application/octet-stream"},
		{Type: provider.ToolContentCustom},
	}}
	cases := []struct {
		name    string
		message provider.Message
	}{
		{name: "user text", message: provider.NewUserMessage(provider.TextPart("text"))},
		{name: "user file", message: provider.NewUserMessage(provider.FilePart("text/plain", provider.DataContent{Text: "file"}))},
		{name: "assistant text", message: provider.NewAssistantMessage(provider.TextPart("text"))},
		{name: "assistant file", message: provider.NewAssistantMessage(provider.FilePart("text/plain", provider.DataContent{Text: "file"}))},
		{name: "assistant custom", message: provider.NewAssistantMessage(provider.CustomPart("provider.custom"))},
		{name: "assistant reasoning", message: provider.NewAssistantMessage(provider.ReasoningPart("reasoning"))},
		{name: "assistant reasoning file", message: provider.NewAssistantMessage(provider.ReasoningFilePart("image/png", provider.DataContent{Base64: "AQ=="}))},
		{name: "assistant tool call", message: provider.NewAssistantMessage(provider.ToolCallPart("call", "tool", json.RawMessage(`null`)))},
		{name: "assistant tool result", message: provider.NewAssistantMessage(provider.ToolResultPart("call", "tool", textOutput))},
		{name: "tool result", message: provider.NewToolMessage(provider.ToolResultPart("call", "tool", contentOutput))},
		{name: "tool approval response", message: provider.NewToolMessage(provider.ContentPart{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval", Approved: &approved})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options := provider.CallOptions{Prompt: []provider.Message{tc.message}}
			encoded, err := EncodeCallOptions(options)
			require.NoError(t, err)
			decoded, err := decodeCallOptionsJSON(encoded)
			require.NoError(t, err)
			assert.Equal(t, options, decoded)
		})
	}

	system := provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleSystem, Content: []provider.ContentPart{provider.TextPart("a"), provider.TextPart("b")}}}}
	encoded, err := EncodeCallOptions(system)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[{"role":"system","content":"ab"}]}`, string(encoded))
}

func TestCallOptions_EmptyInlineDataAndReasoningFileRestrictions(t *testing.T) {
	emptyData := provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(
		provider.FilePart("application/octet-stream", provider.DataContent{Bytes: []byte{}}),
	)}}
	encoded, err := EncodeCallOptions(emptyData)
	require.NoError(t, err)
	assert.JSONEq(t, `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"data","data":""},"mediaType":"application/octet-stream"}]}]}`, string(encoded))
	decoded, err := decodeCallOptionsJSON(encoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.Prompt[0].Content[0].Data.Bytes)
	assert.Empty(t, decoded.Prompt[0].Content[0].Data.Bytes)

	var emptyURL provider.DataContent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"url","url":""}`), &emptyURL))
	require.True(t, emptyURL.IsURL())
	_, err = EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(
		provider.FilePart("application/octet-stream", emptyURL),
	)}})
	require.Error(t, err)

	for _, data := range []provider.DataContent{
		{Reference: json.RawMessage(`{"provider":"file"}`)},
		{Text: "inline"},
	} {
		_, err := EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{
			provider.NewAssistantMessage(provider.ReasoningFilePart("text/plain", data)),
		}})
		require.Error(t, err)
	}
	for _, variant := range []string{
		`{"type":"reference","reference":{"provider":"file"}}`,
		`{"type":"text","text":"inline"}`,
	} {
		wire := `{"prompt":[{"role":"assistant","content":[{"type":"reasoning-file","data":` + variant + `,"mediaType":"text/plain"}]}]}`
		_, err := decodeCallOptionsJSON([]byte(wire))
		require.Error(t, err)
	}
	for _, filename := range []string{`null`, `""`, `"secret"`} {
		wire := `{"prompt":[{"role":"assistant","content":[{"type":"reasoning-file","data":{"type":"data","data":""},"mediaType":"text/plain","filename":` + filename + `}]}]}`
		_, err := decodeCallOptionsJSON([]byte(wire))
		require.Error(t, err)
	}
}

func TestCallOptions_PrivateFieldsAndRequiredIdentifiers(t *testing.T) {
	for _, field := range []string{"sourceType", "id", "url", "title", "signature", "isAutomatic"} {
		t.Run("decode-content-"+field, func(t *testing.T) {
			wire := `{"prompt":[{"role":"user","content":[{"type":"text","text":"x","` + field + `":null}]}]}`
			_, err := decodeCallOptionsJSON([]byte(wire))
			require.Error(t, err)
		})
	}
	for _, field := range []string{"toolCallId", "toolName", "providerExecuted"} {
		t.Run("decode-approval-"+field, func(t *testing.T) {
			wire := `{"prompt":[{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"approval","approved":false,"` + field + `":null}]}]}`
			_, err := decodeCallOptionsJSON([]byte(wire))
			require.Error(t, err)
		})
	}

	privateParts := []provider.ContentPart{
		{Type: provider.ContentPartTypeText, Text: "x", SourceType: provider.SourceTypeURL},
		{Type: provider.ContentPartTypeText, Text: "x", ID: "id"},
		{Type: provider.ContentPartTypeText, Text: "x", URL: "https://example.com"},
		{Type: provider.ContentPartTypeText, Text: "x", Title: "title"},
		{Type: provider.ContentPartTypeText, Text: "x", Signature: "signature"},
		{Type: provider.ContentPartTypeText, Text: "x", IsAutomatic: true},
	}
	for i, part := range privateParts {
		t.Run(fmt.Sprintf("encode-private-%d", i), func(t *testing.T) {
			_, err := EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(part)}})
			require.Error(t, err)
			_, err = EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleSystem, Content: []provider.ContentPart{part}}}})
			require.Error(t, err)
		})
	}

	approved := false
	privateApprovals := []provider.ContentPart{
		{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval", Approved: &approved, ToolCallID: "call"},
		{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval", Approved: &approved, ToolName: "tool"},
		{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval", Approved: &approved, ProviderExecuted: true},
	}
	for i, part := range privateApprovals {
		t.Run(fmt.Sprintf("encode-private-approval-%d", i), func(t *testing.T) {
			_, err := EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(part)}})
			require.Error(t, err)
		})
	}

	for _, wire := range []string{
		`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"","toolName":"tool","input":{}}]}]}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"","input":{}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"","toolName":"tool","output":{"type":"text","value":"ok"}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"","output":{"type":"text","value":"ok"}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"","approved":false}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"execution-denied","reason":null}}]}]}`,
	} {
		_, err := decodeCallOptionsJSON([]byte(wire))
		require.Error(t, err)
	}
}

func TestCustomContentQualifiedIdentifiers_EncodeAndDecode(t *testing.T) {
	invalid := []string{"", "tool", ".tool", "p.", " .tool", "p. "}
	for _, value := range invalid {
		t.Run(fmt.Sprintf("request-custom-%q", value), func(t *testing.T) {
			_, err := encodeContentPart(provider.ContentPart{Type: provider.ContentPartTypeCustom, Kind: value})
			require.Error(t, err)
			_, err = decodeContentPart(json.RawMessage(`{"type":"custom","kind":` + mustJSON(t, value) + `}`))
			require.Error(t, err)
		})
	}
}
