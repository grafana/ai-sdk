package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextPart(t *testing.T) {
	p := TextPart("hello")
	assert.Equal(t, ContentPartTypeText, p.Type)
	assert.Equal(t, "hello", p.Text)
}

func TestFilePart(t *testing.T) {
	p := FilePart("image/png", DataContent{URL: "https://example.com/x.png"})
	assert.Equal(t, ContentPartTypeFile, p.Type)
	assert.Equal(t, "image/png", p.MediaType)
	require.NotNil(t, p.Data)
	assert.Equal(t, "https://example.com/x.png", p.Data.URL)
}

func TestReasoningPart(t *testing.T) {
	p := ReasoningPart("thinking out loud")
	assert.Equal(t, ContentPartTypeReasoning, p.Type)
	assert.Equal(t, "thinking out loud", p.Text)
}

func TestReasoningFilePart(t *testing.T) {
	p := ReasoningFilePart("image/png", DataContent{Bytes: []byte{1, 2, 3}})
	assert.Equal(t, ContentPartTypeReasoningFile, p.Type)
	assert.Equal(t, "image/png", p.MediaType)
	require.NotNil(t, p.Data)
	assert.Equal(t, []byte{1, 2, 3}, p.Data.Bytes)
}

func TestSourcePart(t *testing.T) {
	p := SourcePart(SourceInfo{
		SourceType: SourceTypeURL,
		ID:         "src_1",
		URL:        "https://example.com",
		Title:      "Example",
		ProviderMetadata: ProviderMetadata{
			"openai": json.RawMessage(`{"itemId":"item_1"}`),
		},
	})
	assert.Equal(t, ContentPartTypeSource, p.Type)
	assert.Equal(t, SourceTypeURL, p.SourceType)
	assert.Equal(t, "src_1", p.ID)
	assert.Equal(t, "https://example.com", p.URL)
	assert.Equal(t, "Example", p.Title)
	require.NotNil(t, p.ProviderOptions)
	data, err := json.Marshal(p.ProviderOptions)
	require.NoError(t, err)
	assert.JSONEq(t, `{"openai":{"itemId":"item_1"}}`, string(data))
}

func TestToolCallPart(t *testing.T) {
	input := json.RawMessage(`{"x":1}`)
	p := ToolCallPart("call_123", "get_weather", input)
	assert.Equal(t, ContentPartTypeToolCall, p.Type)
	assert.Equal(t, "call_123", p.ToolCallID)
	assert.Equal(t, "get_weather", p.ToolName)
	assert.JSONEq(t, `{"x":1}`, string(p.Input))
}

func TestToolResultPart(t *testing.T) {
	out := &ToolResultOutput{Type: ToolOutputText, Text: "sunny"}
	p := ToolResultPart("call_123", "get_weather", out)
	assert.Equal(t, ContentPartTypeToolResult, p.Type)
	assert.Equal(t, "call_123", p.ToolCallID)
	assert.Equal(t, "get_weather", p.ToolName)
	require.NotNil(t, p.Output)
	assert.Equal(t, "sunny", p.Output.Text)
}

func TestCustomPart(t *testing.T) {
	p := CustomPart("anthropic.cache-control")
	assert.Equal(t, ContentPartTypeCustom, p.Type)
	assert.Equal(t, "anthropic.cache-control", p.Kind)
}

func TestToolApprovalRequestPart(t *testing.T) {
	p := ToolApprovalRequestPart("apr_1", "call_123", false)
	assert.Equal(t, ContentPartTypeToolApprovalRequest, p.Type)
	assert.Equal(t, "apr_1", p.ApprovalID)
	assert.Equal(t, "call_123", p.ToolCallID)
	assert.False(t, p.IsAutomatic)
}

func TestToolApprovalResponsePart(t *testing.T) {
	p := ToolApprovalResponsePart("apr_1", true, "looks good")
	assert.Equal(t, ContentPartTypeToolApprovalResponse, p.Type)
	assert.Equal(t, "apr_1", p.ApprovalID)
	require.NotNil(t, p.Approved)
	assert.True(t, *p.Approved)
	require.NotNil(t, p.Reason)
	assert.Equal(t, "looks good", *p.Reason)

	denied := ToolApprovalResponsePart("apr_2", false, "unsafe")
	require.NotNil(t, denied.Approved)
	assert.False(t, *denied.Approved)
	require.NotNil(t, denied.Reason)
	assert.Equal(t, "unsafe", *denied.Reason)

	providerExecuted := ProviderExecutedToolApprovalResponsePart("apr_3", true, "ok")
	require.NotNil(t, providerExecuted.ProviderExecuted)
	assert.True(t, *providerExecuted.ProviderExecuted)
}

func TestUserText(t *testing.T) {
	m := UserText("hi there")
	assert.Equal(t, RoleUser, m.Role)
	require.Len(t, m.Content, 1)
	assert.Equal(t, ContentPartTypeText, m.Content[0].Type)
	assert.Equal(t, "hi there", m.Content[0].Text)
}

func TestAssistantText(t *testing.T) {
	m := AssistantText("hello back")
	assert.Equal(t, RoleAssistant, m.Role)
	require.Len(t, m.Content, 1)
	assert.Equal(t, ContentPartTypeText, m.Content[0].Type)
	assert.Equal(t, "hello back", m.Content[0].Text)
}

// TestHelpers_RoundTripWithMessage verifies helpers compose cleanly with
// NewUserMessage / NewAssistantMessage and survive JSON round-trip.
func TestHelpers_RoundTripWithMessage(t *testing.T) {
	out := &ToolResultOutput{Type: ToolOutputJSON, JSON: json.RawMessage(`{"ok":true}`)}
	msg := NewAssistantMessage(
		TextPart("here is the result:"),
		ReasoningPart("the user asked for x"),
		SourcePart(SourceInfo{SourceType: SourceTypeURL, ID: "src_1", URL: "https://example.com"}),
		ToolCallPart("call_1", "fetch", json.RawMessage(`{"q":"hello"}`)),
		ToolResultPart("call_1", "fetch", out),
	)

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var got Message
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, RoleAssistant, got.Role)
	require.Len(t, got.Content, 5)
	assert.Equal(t, ContentPartTypeText, got.Content[0].Type)
	assert.Equal(t, ContentPartTypeReasoning, got.Content[1].Type)
	assert.Equal(t, ContentPartTypeSource, got.Content[2].Type)
	assert.Equal(t, ContentPartTypeToolCall, got.Content[3].Type)
	assert.Equal(t, ContentPartTypeToolResult, got.Content[4].Type)
}
