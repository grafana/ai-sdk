package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageRoles(t *testing.T) {
	cases := []struct {
		name string
		msg  Message
		want Role
	}{
		{"system", NewSystemMessage("hi"), RoleSystem},
		{"user", NewUserMessage(ContentPart{Type: ContentPartTypeText, Text: "hi"}), RoleUser},
		{"assistant", NewAssistantMessage(ContentPart{Type: ContentPartTypeText, Text: "hi"}), RoleAssistant},
		{"tool", NewToolMessage(ContentPart{Type: ContentPartTypeToolResult, ToolCallID: "1", ToolName: "t"}), RoleTool},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.msg.Role)
		})
	}
}

func TestNewSystemMessage(t *testing.T) {
	m := NewSystemMessage("hello")
	assert.Equal(t, RoleSystem, m.Role)
	require.Len(t, m.Content, 1)
	assert.Equal(t, ContentPartTypeText, m.Content[0].Type)
	assert.Equal(t, "hello", m.Content[0].Text)
}

func TestTextParts(t *testing.T) {
	parts := TextParts("hello")
	require.Len(t, parts, 1)
	assert.Equal(t, ContentPartTypeText, parts[0].Type)
	assert.Equal(t, "hello", parts[0].Text)
}

func TestRoleDispatchPattern(t *testing.T) {
	msgs := []Message{
		NewSystemMessage("system"),
		NewUserMessage(ContentPart{Type: ContentPartTypeText, Text: "user"}),
		NewAssistantMessage(
			ContentPart{Type: ContentPartTypeText, Text: "assistant"},
			ContentPart{Type: ContentPartTypeToolCall, ToolName: "weather"},
		),
		NewToolMessage(ContentPart{Type: ContentPartTypeToolResult, ToolCallID: "1", ToolName: "weather"}),
	}

	roles := make([]Role, len(msgs))
	for i, msg := range msgs {
		roles[i] = msg.Role
	}

	expected := []Role{RoleSystem, RoleUser, RoleAssistant, RoleTool}
	assert.Equal(t, expected, roles)
}

func TestMessageJSONRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		msg  Message
	}{
		{
			name: "system",
			msg:  NewSystemMessage("hello"),
		},
		{
			name: "user with text and file",
			msg: NewUserMessage(
				ContentPart{Type: ContentPartTypeText, Text: "describe"},
				ContentPart{Type: ContentPartTypeFile, MediaType: "image/png", Data: &DataContent{URL: "https://example.com/img.png"}},
			),
		},
		{
			name: "assistant with text + tool call",
			msg: NewAssistantMessage(
				ContentPart{Type: ContentPartTypeText, Text: "calling tool"},
				ContentPart{Type: ContentPartTypeToolCall, ToolCallID: "tc_1", ToolName: "search", Input: json.RawMessage(`{"q":"go"}`)},
			),
		},
		{
			name: "tool with result + approval response",
			msg: NewToolMessage(
				ContentPart{Type: ContentPartTypeToolResult, ToolCallID: "tc_1", ToolName: "search", Output: &ToolResultOutput{Type: ToolOutputText, Text: "ok"}},
				ContentPart{Type: ContentPartTypeToolApprovalResponse, ApprovalID: "apr_1", Approved: boolPtr(true)},
			),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.msg)
			require.NoError(t, err)

			var decoded Message
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, tc.msg, decoded)
		})
	}
}
