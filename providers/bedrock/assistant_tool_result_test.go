package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func providerExecutedCall(id string, input string) provider.ContentPart {
	part := provider.ToolCallPart(id, "add", json.RawMessage(input))
	part.ProviderExecuted = true
	return part
}

func jsonToolResult(id string, value string) provider.ContentPart {
	return provider.ToolResultPart(id, "add", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(value)})
}

func toolUseContent(id string, input string) contentBlock {
	return contentBlock{ToolUse: &toolUseBlock{ToolUseID: id, Name: "add", Input: json.RawMessage(input)}}
}

func toolResultContentBlock(id string, text string) contentBlock {
	return contentBlock{ToolResult: &toolResultBlock{ToolUseID: id, Content: []toolResultContent{{Text: new(text)}}}}
}

func TestConvertPrompt_AssistantToolResults(t *testing.T) {
	cache := &cachePoint{Type: "default", TTL: "5m"}
	cases := []struct {
		name   string
		prompt []provider.Message
		want   []converseMessage
	}{
		{
			name: "preserves order of provider-executed calls and results",
			prompt: []provider.Message{
				provider.UserText("Add 2 and 2, then add 3 and 3."),
				provider.NewAssistantMessage(
					provider.TextPart("Running the additions."),
					providerExecutedCall("call-1", `{"a":2,"b":2}`),
					jsonToolResult("call-1", `{"sum":4}`),
					providerExecutedCall("call-2", `{"a":3,"b":3}`),
					jsonToolResult("call-2", `{"sum":6}`),
					provider.TextPart("The sums are 4 and 6."),
				),
				provider.UserText("Now add those sums together."),
			},
			want: []converseMessage{
				{Role: converseRoleUser, Content: []contentBlock{{Text: "Add 2 and 2, then add 3 and 3."}}},
				{Role: converseRoleAssistant, Content: []contentBlock{{Text: "Running the additions."}, toolUseContent("call-1", `{"a":2,"b":2}`)}},
				{Role: converseRoleUser, Content: []contentBlock{toolResultContentBlock("call-1", `{"sum":4}`)}},
				{Role: converseRoleAssistant, Content: []contentBlock{toolUseContent("call-2", `{"a":3,"b":3}`)}},
				{Role: converseRoleUser, Content: []contentBlock{toolResultContentBlock("call-2", `{"sum":6}`)}},
				{Role: converseRoleAssistant, Content: []contentBlock{{Text: "The sums are 4 and 6."}}},
				{Role: converseRoleUser, Content: []contentBlock{{Text: "Now add those sums together."}}},
			},
		},
		{
			name: "combines trailing result with next user message",
			prompt: []provider.Message{
				provider.NewAssistantMessage(providerExecutedCall("call-1", `{"a":2,"b":2}`), jsonToolResult("call-1", `{"sum":4}`)),
				provider.UserText("Now double that."),
			},
			want: []converseMessage{
				{Role: converseRoleAssistant, Content: []contentBlock{toolUseContent("call-1", `{"a":2,"b":2}`)}},
				{Role: converseRoleUser, Content: []contentBlock{toolResultContentBlock("call-1", `{"sum":4}`), {Text: "Now double that."}}},
			},
		},
		{
			name: "leading result joins previous tool results",
			prompt: []provider.Message{
				provider.NewToolMessage(jsonToolResult("call-0", `{"sum":1}`)),
				provider.NewAssistantMessage(jsonToolResult("call-1", `{"sum":4}`), provider.TextPart("done")),
			},
			want: []converseMessage{
				{Role: converseRoleUser, Content: []contentBlock{toolResultContentBlock("call-0", `{"sum":1}`), toolResultContentBlock("call-1", `{"sum":4}`)}},
				{Role: converseRoleAssistant, Content: []contentBlock{{Text: "done"}}},
			},
		},
		{
			name: "leading result after plain user text starts a user message",
			prompt: []provider.Message{
				provider.UserText("question"),
				provider.NewAssistantMessage(jsonToolResult("call-1", `{"sum":4}`), provider.TextPart("done")),
			},
			want: []converseMessage{
				{Role: converseRoleUser, Content: []contentBlock{{Text: "question"}}},
				{Role: converseRoleUser, Content: []contentBlock{toolResultContentBlock("call-1", `{"sum":4}`)}},
				{Role: converseRoleAssistant, Content: []contentBlock{{Text: "done"}}},
			},
		},
		{
			name: "user content after omitted assistant joins previous tool results",
			prompt: []provider.Message{
				provider.NewToolMessage(jsonToolResult("call-0", `{"sum":1}`)),
				provider.NewAssistantMessage(provider.ReasoningPart("unsigned")),
				provider.UserText("next"),
			},
			want: []converseMessage{
				{Role: converseRoleUser, Content: []contentBlock{toolResultContentBlock("call-0", `{"sum":1}`), {Text: "next"}}},
			},
		},
		{
			name: "message cache point follows trailing tool result",
			prompt: []provider.Message{
				cachedMessage(provider.RoleAssistant, providerExecutedCall("call-1", `{}`), jsonToolResult("call-1", `{"sum":4}`)),
				provider.UserText("next"),
			},
			want: []converseMessage{
				{Role: converseRoleAssistant, Content: []contentBlock{toolUseContent("call-1", `{}`)}},
				{Role: converseRoleUser, Content: []contentBlock{toolResultContentBlock("call-1", `{"sum":4}`), {CachePoint: cache}, {Text: "next"}}},
			},
		},
		{
			name: "message cache point stays on trailing assistant content",
			prompt: []provider.Message{
				cachedMessage(provider.RoleAssistant, providerExecutedCall("call-1", `{}`), jsonToolResult("call-1", `{"sum":4}`), provider.TextPart("done")),
				provider.UserText("next"),
			},
			want: []converseMessage{
				{Role: converseRoleAssistant, Content: []contentBlock{toolUseContent("call-1", `{}`)}},
				{Role: converseRoleUser, Content: []contentBlock{toolResultContentBlock("call-1", `{"sum":4}`)}},
				{Role: converseRoleAssistant, Content: []contentBlock{{Text: "done"}, {CachePoint: cache}}},
				{Role: converseRoleUser, Content: []contentBlock{{Text: "next"}}},
			},
		},
		{
			name: "message cache point is dropped when trailing assistant part emits nothing",
			prompt: []provider.Message{
				cachedMessage(provider.RoleAssistant, providerExecutedCall("call-1", `{}`), jsonToolResult("call-1", `{"sum":4}`), provider.ReasoningPart("unsigned")),
				provider.UserText("next"),
			},
			want: []converseMessage{
				{Role: converseRoleAssistant, Content: []contentBlock{toolUseContent("call-1", `{}`)}},
				{Role: converseRoleUser, Content: []contentBlock{toolResultContentBlock("call-1", `{"sum":4}`), {Text: "next"}}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, warnings, err := convertPrompt(tc.prompt, false, true)
			require.NoError(t, err)
			assert.Empty(t, warnings)
			assert.Empty(t, got.System)
			assert.Equal(t, tc.want, got.Messages)
		})
	}
}

func TestConvertPrompt_AssistantToolResultConversionRules(t *testing.T) {
	t.Run("normalizes mistral tool call ids", func(t *testing.T) {
		got, _, err := convertPrompt([]provider.Message{provider.NewAssistantMessage(jsonToolResult("call-with-long-id", `{}`))}, true, true)
		require.NoError(t, err)
		require.Len(t, got.Messages, 1)
		assert.Equal(t, normalizeToolCallID("call-with-long-id", true), got.Messages[0].Content[0].ToolResult.ToolUseID)
	})

	t.Run("shares document counter with user documents", func(t *testing.T) {
		document := provider.ToolResultPart("call-1", "add", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{
			Type: provider.ToolContentFile, MediaType: "text/plain", Data: &provider.DataContent{Base64: "aGk="},
		}}})
		got, _, err := convertPrompt([]provider.Message{
			provider.NewUserMessage(provider.FilePart("text/plain", provider.DataContent{Base64: "aGk="})),
			provider.NewAssistantMessage(document),
		}, false, true)
		require.NoError(t, err)
		require.Len(t, got.Messages, 2)
		assert.Equal(t, "document-1", got.Messages[0].Content[0].Document.Name)
		assert.Equal(t, "document-2", got.Messages[1].Content[0].ToolResult.Content[0].Document.Name)
	})

	t.Run("rejects custom tool content", func(t *testing.T) {
		custom := provider.ToolResultPart("call-1", "add", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentCustom}}})
		_, _, err := convertPrompt([]provider.Message{provider.NewAssistantMessage(custom)}, false, true)
		require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
		assert.Contains(t, err.Error(), "unsupported tool content part type: custom")
	})

	t.Run("removes results without active tools", func(t *testing.T) {
		got, warnings, err := convertPrompt([]provider.Message{
			provider.NewAssistantMessage(jsonToolResult("call-1", `{}`), provider.TextPart("done")),
		}, false, false)
		require.NoError(t, err)
		assert.Equal(t, []converseMessage{{Role: converseRoleAssistant, Content: []contentBlock{{Text: "done"}}}}, got.Messages)
		assert.Equal(t, []string{"toolContent"}, warningFeatures(warnings))
	})
}
