package aisdk

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var cacheMeta = provider.ProviderMetadata{"anthropic": json.RawMessage(`{"cacheControl":{"type":"ephemeral"}}`)}

func convert(t *testing.T, msgs []UIMessage, opts ...ConvertOption) []provider.Message {
	t.Helper()
	result, err := ConvertToModelMessages(msgs, opts...)
	require.NoError(t, err)
	return result
}

// systemText returns the joined text of a system-role message constructed by
// ConvertToModelMessages, which packs the system prompt into a single text
// content part.
func systemText(m provider.Message) string {
	if len(m.Content) == 0 {
		return ""
	}
	return m.Content[0].Text
}

func TestConvertToModelMessages_System(t *testing.T) {
	tests := []struct {
		name  string
		parts []Part
		check func(t *testing.T, result []provider.Message)
	}{
		{
			name:  "single text part",
			parts: []Part{TextPart{Text: "be helpful"}},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 1)
				assert.Equal(t, provider.RoleSystem, result[0].Role)
				assert.Equal(t, "be helpful", systemText(result[0]))
			},
		},
		{
			name: "joins multiple text parts",
			parts: []Part{
				TextPart{Text: "You are helpful."},
				TextPart{Text: " Be concise."},
			},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 1)
				assert.Equal(t, "You are helpful. Be concise.", systemText(result[0]))
			},
		},
		{
			name: "aggregates providerMetadata from text parts",
			parts: []Part{
				TextPart{Text: "first", ProviderMetadata: cacheMeta},
				TextPart{Text: "second", ProviderMetadata: provider.ProviderMetadata{"google": json.RawMessage(`{"key":"val"}`)}},
			},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 1)
				assert.Equal(t, "firstsecond", systemText(result[0]))
				assert.Contains(t, result[0].ProviderOptions, "anthropic")
				assert.Contains(t, result[0].ProviderOptions, "google")
			},
		},
		{
			name:  "empty text produces no message",
			parts: []Part{TextPart{Text: ""}},
			check: func(t *testing.T, result []provider.Message) {
				assert.Empty(t, result)
			},
		},
		{
			name:  "empty text with providerMetadata preserves system message",
			parts: []Part{TextPart{Text: "", ProviderMetadata: cacheMeta}},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 1)
				assert.Equal(t, provider.RoleSystem, result[0].Role)
				assert.Empty(t, systemText(result[0]))
				assert.Contains(t, result[0].ProviderOptions, "anthropic")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs := []UIMessage{{ID: "1", Role: RoleSystem, Parts: tc.parts}}
			result := convert(t, msgs)
			tc.check(t, result)
		})
	}
}

func TestConvertToModelMessages_User(t *testing.T) {
	tests := []struct {
		name  string
		parts []Part
		check func(t *testing.T, um provider.Message)
	}{
		{
			name:  "text part",
			parts: []Part{TextPart{Text: "hello"}},
			check: func(t *testing.T, um provider.Message) {
				require.Len(t, um.Content, 1)
				assert.Equal(t, provider.ContentPartTypeText, um.Content[0].Type)
				assert.Equal(t, "hello", um.Content[0].Text)
			},
		},
		{
			name:  "text part carries providerMetadata",
			parts: []Part{TextPart{Text: "hello", ProviderMetadata: cacheMeta}},
			check: func(t *testing.T, um provider.Message) {
				assert.Contains(t, um.Content[0].ProviderOptions, "anthropic")
			},
		},
		{
			name:  "file part",
			parts: []Part{FilePart{MediaType: "image/png", URL: "https://example.com/img.png"}},
			check: func(t *testing.T, um provider.Message) {
				cp := um.Content[0]
				assert.Equal(t, provider.ContentPartTypeFile, cp.Type)
				require.NotNil(t, cp.Data)
				assert.Equal(t, "https://example.com/img.png", cp.Data.URL)
			},
		},
		{
			name: "provider reference takes precedence over file URL",
			parts: []Part{FilePart{
				MediaType:         "application/pdf",
				URL:               "data:application/pdf;base64,abc123",
				ProviderReference: map[string]string{"openai": "file-abc123"},
			}},
			check: func(t *testing.T, um provider.Message) {
				cp := um.Content[0]
				require.NotNil(t, cp.Data)
				assert.JSONEq(t, `{"openai":"file-abc123"}`, string(cp.Data.Reference))
				assert.Empty(t, cp.Data.Base64)
				assert.Empty(t, cp.Data.URL)
			},
		},
		{
			name: "empty provider reference still takes precedence over file URL",
			parts: []Part{FilePart{
				MediaType:         "application/pdf",
				URL:               "https://example.com/doc.pdf",
				ProviderReference: map[string]string{},
			}},
			check: func(t *testing.T, um provider.Message) {
				cp := um.Content[0]
				require.NotNil(t, cp.Data)
				assert.NotNil(t, cp.Data.Reference)
				assert.Empty(t, cp.Data.URL)
			},
		},
		{
			name:  "data URL file part becomes base64 data",
			parts: []Part{FilePart{MediaType: "image/*", URL: "data:image/png;base64,abc123"}},
			check: func(t *testing.T, um provider.Message) {
				cp := um.Content[0]
				assert.Equal(t, provider.ContentPartTypeFile, cp.Type)
				require.NotNil(t, cp.Data)
				assert.Equal(t, "image/png", cp.MediaType)
				assert.Equal(t, "abc123", cp.Data.Base64)
				assert.Empty(t, cp.Data.URL)
			},
		},
		{
			name:  "file part carries providerMetadata",
			parts: []Part{FilePart{MediaType: "image/png", URL: "https://example.com/img.png", ProviderMetadata: cacheMeta}},
			check: func(t *testing.T, um provider.Message) {
				assert.Contains(t, um.Content[0].ProviderOptions, "anthropic")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs := []UIMessage{{ID: "1", Role: RoleUser, Parts: tc.parts}}
			result := convert(t, msgs)
			require.Len(t, result, 1)
			assert.Equal(t, provider.RoleUser, result[0].Role)
			tc.check(t, result[0])
		})
	}
}

func TestConvertToModelMessages_AssistantContent(t *testing.T) {
	tests := []struct {
		name  string
		parts []Part
		check func(t *testing.T, am provider.Message)
	}{
		{
			name:  "text part",
			parts: []Part{TextPart{Text: "hi"}},
			check: func(t *testing.T, am provider.Message) {
				require.Len(t, am.Content, 1)
				assert.Equal(t, provider.ContentPartTypeText, am.Content[0].Type)
				assert.Equal(t, "hi", am.Content[0].Text)
			},
		},
		{
			name:  "text part carries providerMetadata",
			parts: []Part{TextPart{Text: "response", ProviderMetadata: cacheMeta}},
			check: func(t *testing.T, am provider.Message) {
				assert.Contains(t, am.Content[0].ProviderOptions, "anthropic")
			},
		},
		{
			name:  "empty text is skipped",
			parts: []Part{TextPart{Text: ""}, TextPart{Text: "real"}},
			check: func(t *testing.T, am provider.Message) {
				require.Len(t, am.Content, 1)
			},
		},
		{
			name:  "reasoning part with providerMetadata",
			parts: []Part{ReasoningPart{Text: "thinking...", ProviderMetadata: cacheMeta}},
			check: func(t *testing.T, am provider.Message) {
				cp := am.Content[0]
				assert.Equal(t, provider.ContentPartTypeReasoning, cp.Type)
				assert.Equal(t, "thinking...", cp.Text)
				assert.Contains(t, cp.ProviderOptions, "anthropic")
			},
		},
		{
			name:  "file part carries providerMetadata",
			parts: []Part{FilePart{MediaType: "image/png", URL: "https://example.com/img.png", ProviderMetadata: cacheMeta}},
			check: func(t *testing.T, am provider.Message) {
				assert.Equal(t, provider.ContentPartTypeFile, am.Content[0].Type)
				assert.Contains(t, am.Content[0].ProviderOptions, "anthropic")
			},
		},
		{
			name:  "reasoning file part carries providerMetadata",
			parts: []Part{ReasoningFilePart{MediaType: "image/png", URL: "https://example.com/reasoning.png", ProviderMetadata: cacheMeta}},
			check: func(t *testing.T, am provider.Message) {
				part := am.Content[0]
				assert.Equal(t, provider.ContentPartTypeReasoningFile, part.Type)
				require.NotNil(t, part.Data)
				assert.Equal(t, "https://example.com/reasoning.png", part.Data.URL)
				assert.Equal(t, "image/png", part.MediaType)
				assert.Contains(t, part.ProviderOptions, "anthropic")
			},
		},
		{
			name:  "file data URL remains a URL",
			parts: []Part{FilePart{MediaType: "image/png", URL: "data:image/png;base64,abc123"}},
			check: func(t *testing.T, am provider.Message) {
				require.NotNil(t, am.Content[0].Data)
				assert.Equal(t, "data:image/png;base64,abc123", am.Content[0].Data.URL)
				assert.Empty(t, am.Content[0].Data.Base64)
			},
		},
		{
			name: "file part preserves provider reference",
			parts: []Part{FilePart{
				MediaType:         "application/pdf",
				URL:               "data:application/pdf;base64,xyz",
				ProviderReference: map[string]string{"anthropic": "file-xyz789"},
			}},
			check: func(t *testing.T, am provider.Message) {
				require.NotNil(t, am.Content[0].Data)
				assert.JSONEq(t, `{"anthropic":"file-xyz789"}`, string(am.Content[0].Data.Reference))
				assert.Empty(t, am.Content[0].Data.Base64)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: tc.parts}}
			result := convert(t, msgs)
			require.Len(t, result, 1)
			assert.Equal(t, provider.RoleAssistant, result[0].Role)
			tc.check(t, result[0])
		})
	}
}

func TestConvertToModelMessages_CustomPart(t *testing.T) {
	metadata := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"cmp-1"}`)}
	messages, err := ConvertToModelMessages([]UIMessage{{
		Role: RoleAssistant,
		Parts: []Part{CustomPart{
			Kind:             "openai.compaction",
			ProviderMetadata: metadata,
		}},
	}})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Len(t, messages[0].Content, 1)
	part := messages[0].Content[0]
	assert.Equal(t, provider.ContentPartTypeCustom, part.Type)
	assert.Equal(t, "openai.compaction", part.Kind)
	assert.Equal(t, providerMetadataToOptions(metadata), part.ProviderOptions)
}

func TestConvertToModelMessages_Tools(t *testing.T) {
	approved := true
	tests := []struct {
		name  string
		parts []Part
		opts  []ConvertOption
		check func(t *testing.T, result []provider.Message)
	}{
		{
			name: "tool invocation produces assistant + tool messages",
			parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "weather", State: ToolStateOutputAvailable,
				Input: json.RawMessage(`{"city":"NYC"}`), Output: json.RawMessage(`{"temp":72}`),
			}},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 2)
				am := result[0]
				assert.Equal(t, provider.RoleAssistant, am.Role)
				tc := am.Content[0]
				assert.Equal(t, provider.ContentPartTypeToolCall, tc.Type)
				assert.Equal(t, "weather", tc.ToolName)
				tm := result[1]
				assert.Equal(t, provider.RoleTool, tm.Role)
				tr := tm.Content[0]
				assert.Equal(t, provider.ContentPartTypeToolResult, tr.Type)
				assert.Equal(t, "c1", tr.ToolCallID)
			},
		},
		{
			name: "dynamic tool output error",
			parts: []Part{DynamicToolUIPart{
				ToolCallID: "c1", ToolName: "mcp_search", State: ToolStateOutputError,
				ErrorText: "connection timeout",
			}},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 2)
				tr := result[1].Content[0]
				require.NotNil(t, tr.Output)
				assert.Equal(t, provider.ToolOutputErrorText, tr.Output.Type)
				assert.Equal(t, "connection timeout", tr.Output.Text)
			},
		},
		{
			name: "incomplete tool calls filtered when option set",
			parts: []Part{
				TextPart{Text: "let me check"},
				ToolInvocationPart{ToolCallID: "c1", ToolName: "weather", State: ToolStateInputStreaming},
			},
			opts: []ConvertOption{WithIgnoreIncompleteToolCalls()},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 1)
				assert.Len(t, result[0].Content, 1, "expected text only")
			},
		},
		{
			name: "approval-requested survives incomplete filter",
			parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "weather", State: ToolStateApprovalRequested,
				Input:    json.RawMessage(`{"city":"NYC"}`),
				Approval: &ToolApproval{ID: "apr_1"},
			}},
			opts: []ConvertOption{WithIgnoreIncompleteToolCalls()},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 1)
				require.Len(t, result[0].Content, 2)
				assert.Equal(t, provider.ContentPartTypeToolCall, result[0].Content[0].Type)
				assert.Equal(t, provider.ContentPartTypeToolApprovalRequest, result[0].Content[1].Type)
				assert.Equal(t, "apr_1", result[0].Content[1].ApprovalID)
			},
		},
		{
			name: "approval-responded survives incomplete filter",
			parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "weather", State: ToolStateApprovalResponded,
				Input:    json.RawMessage(`{"city":"NYC"}`),
				Approval: &ToolApproval{ID: "apr_1", Approved: &approved, Reason: "ok"},
			}},
			opts: []ConvertOption{WithIgnoreIncompleteToolCalls()},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 2)
				require.Len(t, result[0].Content, 2)
				assert.Equal(t, provider.ContentPartTypeToolApprovalRequest, result[0].Content[1].Type)
				require.Len(t, result[1].Content, 1)
				assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, result[1].Content[0].Type)
				require.NotNil(t, result[1].Content[0].Approved)
				assert.True(t, *result[1].Content[0].Approved)
			},
		},
		{
			name: "tool call/result carries providerMetadata",
			parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "weather", State: ToolStateOutputAvailable,
				Input: json.RawMessage(`{}`), Output: json.RawMessage(`{"temp":72}`),
				CallProviderMetadata:   cacheMeta,
				ResultProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"custom":"data"}`)},
			}},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 2)
				assert.Contains(t, result[0].Content[0].ProviderOptions, "anthropic")
				assert.Contains(t, result[1].Content[0].ProviderOptions, "anthropic")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: tc.parts}}
			result := convert(t, msgs, tc.opts...)
			tc.check(t, result)
		})
	}
}

func TestConvertToModelMessages_ToolModelOutput(t *testing.T) {
	contentOutput := &provider.ToolResultOutput{
		Type: provider.ToolOutputContent,
		Content: []provider.ToolResultContentValue{
			{Type: provider.ToolContentText, Text: "weather report"},
			{Type: provider.ToolContentFileData, Data: "aGVsbG8=", MediaType: "image/png"},
		},
	}

	for _, providerExecuted := range []bool{false, true} {
		name := "client-executed"
		if providerExecuted {
			name = "provider-executed"
		}
		t.Run(name, func(t *testing.T) {
			var got ToolOutputContext
			tools := ToolSet{"weather": {
				ToModelOutput: func(ctx ToolOutputContext) (*provider.ToolResultOutput, error) {
					got = ctx
					return contentOutput, nil
				},
			}}
			msgs := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{
				ToolCallID:       "c1",
				ToolName:         "weather",
				State:            ToolStateOutputAvailable,
				Input:            json.RawMessage(`{"city":"NYC"}`),
				Output:           json.RawMessage(`{"temp":72}`),
				ProviderExecuted: providerExecuted,
			}}}}

			result := convert(t, msgs, WithTools(tools))
			assert.Equal(t, "c1", got.ToolCallID)
			assert.JSONEq(t, `{"city":"NYC"}`, string(got.Input))
			assert.JSONEq(t, `{"temp":72}`, string(got.Output))

			resultMessage := result[len(result)-1]
			toolResult := resultMessage.Content[len(resultMessage.Content)-1]
			require.Same(t, contentOutput, toolResult.Output)
		})
	}

	t.Run("string fallback uses text output", func(t *testing.T) {
		msgs := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{
			ToolCallID: "c1", ToolName: "weather", State: ToolStateOutputAvailable,
			Input: json.RawMessage(`{}`), Output: json.RawMessage(`"sunny"`),
		}}}}
		result := convert(t, msgs)
		output := result[1].Content[0].Output
		require.NotNil(t, output)
		assert.Equal(t, provider.ToolOutputText, output.Type)
		assert.Equal(t, "sunny", output.Text)
	})

	nullOutput := json.RawMessage(" \nnull\t")
	for _, providerExecuted := range []bool{false, true} {
		name := "client-executed"
		if providerExecuted {
			name = "provider-executed"
		}
		t.Run("null fallback uses JSON output/"+name, func(t *testing.T) {
			msgs := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "weather", State: ToolStateOutputAvailable,
				Input: json.RawMessage(`{}`), Output: nullOutput,
				ProviderExecuted: providerExecuted,
			}}}}
			result := convert(t, msgs)
			resultMessage := result[len(result)-1]
			output := resultMessage.Content[len(resultMessage.Content)-1].Output
			require.NotNil(t, output)
			assert.Equal(t, provider.ToolOutputJSON, output.Type)
			assert.Equal(t, nullOutput, output.JSON)
		})
	}

	t.Run("conversion error is returned", func(t *testing.T) {
		expected := fmt.Errorf("unsupported output")
		msgs := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{
			ToolCallID: "c1", ToolName: "weather", State: ToolStateOutputAvailable,
			Input: json.RawMessage(`{}`), Output: json.RawMessage(`{}`),
		}}}}
		_, err := ConvertToModelMessages(msgs, WithTools(ToolSet{"weather": {
			ToModelOutput: func(ToolOutputContext) (*provider.ToolResultOutput, error) {
				return nil, expected
			},
		}}))
		require.ErrorIs(t, err, expected)
		assert.Contains(t, err.Error(), `converting output for tool "weather"`)
	})
}

func TestConvertToModelMessages_ProviderExecutedTools(t *testing.T) {
	tests := []struct {
		name  string
		parts []Part
		check func(t *testing.T, result []provider.Message)
	}{
		{
			name: "result goes inline in assistant message",
			parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "computer", State: ToolStateOutputAvailable,
				Input: json.RawMessage(`{"action":"screenshot"}`), Output: json.RawMessage(`{"screenshot":"data"}`),
				ProviderExecuted: true,
			}},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 1)
				am := result[0]
				require.Len(t, am.Content, 2, "expected call + result")
				assert.Equal(t, provider.ContentPartTypeToolCall, am.Content[0].Type)
				assert.True(t, am.Content[0].ProviderExecuted)
				tr := am.Content[1]
				assert.Equal(t, provider.ContentPartTypeToolResult, tr.Type)
				assert.Equal(t, "c1", tr.ToolCallID)
				require.NotNil(t, tr.Output)
				assert.Equal(t, provider.ToolOutputJSON, tr.Output.Type)
			},
		},
		{
			name: "error uses error-json format",
			parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "computer", State: ToolStateOutputError,
				Input: json.RawMessage(`{"action":"click"}`), ErrorText: "element not found",
				ProviderExecuted: true,
			}},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 1)
				tr := result[0].Content[1]
				require.NotNil(t, tr.Output)
				assert.Equal(t, provider.ToolOutputErrorJSON, tr.Output.Type)
				var errStr string
				require.NoError(t, json.Unmarshal(tr.Output.JSON, &errStr))
				assert.Equal(t, "element not found", errStr)
			},
		},
		{
			name: "carries providerMetadata inline",
			parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "computer", State: ToolStateOutputAvailable,
				Input: json.RawMessage(`{}`), Output: json.RawMessage(`{"ok":true}`),
				ProviderExecuted:       true,
				CallProviderMetadata:   cacheMeta,
				ResultProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"custom":"data"}`)},
			}},
			check: func(t *testing.T, result []provider.Message) {
				am := result[0]
				assert.Contains(t, am.Content[0].ProviderOptions, "anthropic")
				assert.Contains(t, am.Content[1].ProviderOptions, "anthropic")
			},
		},
		{
			name: "mixed with non-provider-executed tools",
			parts: []Part{
				ToolInvocationPart{
					ToolCallID: "c1", ToolName: "computer", State: ToolStateOutputAvailable,
					Input: json.RawMessage(`{}`), Output: json.RawMessage(`{"ok":true}`),
					ProviderExecuted: true,
				},
				ToolInvocationPart{
					ToolCallID: "c2", ToolName: "weather", State: ToolStateOutputAvailable,
					Input: json.RawMessage(`{}`), Output: json.RawMessage(`{"temp":72}`),
				},
			},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 2)
				am := result[0]
				require.Len(t, am.Content, 3, "expected call+result+call")
				assert.Equal(t, provider.ContentPartTypeToolCall, am.Content[0].Type)
				assert.Equal(t, provider.ContentPartTypeToolResult, am.Content[1].Type)
				assert.Equal(t, provider.ContentPartTypeToolCall, am.Content[2].Type)
				tm := result[1]
				require.Len(t, tm.Content, 1)
				assert.Equal(t, "c2", tm.Content[0].ToolCallID)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: tc.parts}}
			result := convert(t, msgs)
			tc.check(t, result)
		})
	}
}

func TestConvertToModelMessages_OutputDenied(t *testing.T) {
	tests := []struct {
		name  string
		parts []Part
		opts  []ConvertOption
		check func(t *testing.T, result []provider.Message)
	}{
		{
			name: "default denial reason",
			parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "dangerous", State: ToolStateOutputDenied,
				Input: json.RawMessage(`{}`),
			}},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 2)
				tr := result[1].Content[0]
				require.NotNil(t, tr.Output)
				assert.Equal(t, provider.ToolOutputErrorText, tr.Output.Type)
				assert.Equal(t, "Tool execution denied.", tr.Output.Text)
			},
		},
		{
			name: "custom denial reason from approval",
			parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "dangerous", State: ToolStateOutputDenied,
				Input:    json.RawMessage(`{}`),
				Approval: &ToolApproval{Reason: "User rejected this action"},
			}},
			check: func(t *testing.T, result []provider.Message) {
				tr := result[1].Content[0]
				require.NotNil(t, tr.Output)
				assert.Equal(t, "User rejected this action", tr.Output.Text)
			},
		},
		{
			name: "not filtered by IgnoreIncompleteToolCalls",
			parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "dangerous", State: ToolStateOutputDenied,
				Input: json.RawMessage(`{}`),
			}},
			opts: []ConvertOption{WithIgnoreIncompleteToolCalls()},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 2)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: tc.parts}}
			result := convert(t, msgs, tc.opts...)
			tc.check(t, result)
		})
	}
}

// TestConvertToModelMessages_ApprovalResponses asserts that converting a
// UIMessage with an approval response emits a provider tool-approval-response
// content part. This used to be silently dropped, leaving downstream
// providers unable to see the user's decision in follow-up turns.
// Mirrors upstream convert-to-model-messages.ts:289-301.
func TestConvertToModelMessages_ApprovalResponses(t *testing.T) {
	approved := true
	denied := false

	tests := []struct {
		name  string
		parts []Part
		check func(t *testing.T, result []provider.Message)
	}{
		{
			name: "approved client-executed tool emits approval-response and result",
			parts: []Part{ToolInvocationPart{
				ToolCallID: "c1", ToolName: "search",
				State:    ToolStateOutputAvailable,
				Input:    json.RawMessage(`{"q":"go"}`),
				Output:   json.RawMessage(`["result"]`),
				Approval: &ToolApproval{ID: "apr_1", Approved: &approved, Reason: "ok"},
			}},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 2)
				assistMsg := result[0]
				require.Len(t, assistMsg.Content, 2)
				assert.Equal(t, provider.ContentPartTypeToolApprovalRequest, assistMsg.Content[1].Type)
				assert.Equal(t, "apr_1", assistMsg.Content[1].ApprovalID)
				assert.Equal(t, "c1", assistMsg.Content[1].ToolCallID)
				toolMsg := result[1]
				require.Equal(t, provider.RoleTool, toolMsg.Role)
				require.Len(t, toolMsg.Content, 2, "tool message must carry both approval-response and tool-result")
				assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, toolMsg.Content[0].Type)
				assert.Equal(t, "apr_1", toolMsg.Content[0].ApprovalID)
				require.NotNil(t, toolMsg.Content[0].Approved)
				assert.True(t, *toolMsg.Content[0].Approved)
				assert.Equal(t, provider.ContentPartTypeToolResult, toolMsg.Content[1].Type)
			},
		},
		{
			name: "denied provider-executed tool emits approval-response and synthetic execution-denied",
			parts: []Part{ToolInvocationPart{
				ToolCallID:       "c1",
				ToolName:         "dangerous",
				State:            ToolStateApprovalResponded,
				ProviderExecuted: true,
				Input:            json.RawMessage(`{}`),
				Approval:         &ToolApproval{ID: "apr_2", Approved: &denied, Reason: "user denied"},
			}},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 2)
				assistMsg := result[0]
				require.Len(t, assistMsg.Content, 2)
				assert.Equal(t, provider.ContentPartTypeToolApprovalRequest, assistMsg.Content[1].Type)
				assert.Equal(t, "apr_2", assistMsg.Content[1].ApprovalID)
				toolMsg := result[1]
				require.Equal(t, provider.RoleTool, toolMsg.Role)
				require.Len(t, toolMsg.Content, 2)
				assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, toolMsg.Content[0].Type)
				assert.Equal(t, "apr_2", toolMsg.Content[0].ApprovalID)
				require.NotNil(t, toolMsg.Content[0].Approved)
				assert.False(t, *toolMsg.Content[0].Approved)
				assert.True(t, toolMsg.Content[0].ProviderExecuted)
				assert.Equal(t, provider.ContentPartTypeToolResult, toolMsg.Content[1].Type)
				require.NotNil(t, toolMsg.Content[1].Output)
				assert.Equal(t, provider.ToolOutputExecutionDenied, toolMsg.Content[1].Output.Type)
				assert.Equal(t, "user denied", toolMsg.Content[1].Output.Reason)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: tc.parts}}
			result := convert(t, msgs)
			tc.check(t, result)
		})
	}
}

// TestConvertToModelMessages_CallProviderMetadataFallback asserts that the
// tool-result ProviderOptions fall back to callProviderMetadata when
// resultProviderMetadata is absent. Mirrors upstream
// convert-to-model-messages.ts:231-232; without this fallback, cache-control
// and other provider-side metadata are silently dropped.
func TestConvertToModelMessages_CallProviderMetadataFallback(t *testing.T) {
	msgs := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: []Part{
		ToolInvocationPart{
			ToolCallID:       "c1",
			ToolName:         "search",
			State:            ToolStateOutputAvailable,
			ProviderExecuted: true,
			Input:            json.RawMessage(`{}`),
			Output:           json.RawMessage(`{"hits":1}`),
			CallProviderMetadata: provider.ProviderMetadata{
				"anthropic": json.RawMessage(`{"cacheControl":{"type":"ephemeral"}}`),
			},
			// ResultProviderMetadata intentionally nil; the converter must
			// fall back to CallProviderMetadata for the tool-result options.
		},
	}}}
	result := convert(t, msgs)
	require.Len(t, result, 1, "provider-executed result is inlined into the assistant message")
	assistMsg := result[0]
	require.Len(t, assistMsg.Content, 2)

	toolResult := assistMsg.Content[1]
	require.Equal(t, provider.ContentPartTypeToolResult, toolResult.Type)
	require.NotNil(t, toolResult.ProviderOptions, "result options must inherit from call options when result metadata is absent")
	_, ok := toolResult.ProviderOptions["anthropic"]
	assert.True(t, ok, "anthropic options must round-trip from CallProviderMetadata")
}

func TestConvertToModelMessages_MultiStep(t *testing.T) {
	tests := []struct {
		name  string
		parts []Part
		check func(t *testing.T, result []provider.Message)
	}{
		{
			name: "tool call then text continuation",
			parts: []Part{
				StepStartPart{},
				ReasoningPart{Text: "I should look up the weather"},
				ToolInvocationPart{
					ToolCallID: "c1", ToolName: "weather", State: ToolStateOutputAvailable,
					Input: json.RawMessage(`{"city":"NYC"}`), Output: json.RawMessage(`{"temp":72}`),
				},
				StepStartPart{},
				TextPart{Text: "The temperature in NYC is 72F."},
			},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 3)

				am1 := result[0]
				assert.Equal(t, provider.RoleAssistant, am1.Role)
				require.Len(t, am1.Content, 2, "expected reasoning+call")
				assert.Equal(t, provider.ContentPartTypeReasoning, am1.Content[0].Type)
				assert.Equal(t, provider.ContentPartTypeToolCall, am1.Content[1].Type)
				assert.Equal(t, "weather", am1.Content[1].ToolName)

				assert.Equal(t, provider.RoleTool, result[1].Role)
				tm := result[1]
				require.Len(t, tm.Content, 1)
				assert.Equal(t, "c1", tm.Content[0].ToolCallID)

				am2 := result[2]
				assert.Equal(t, provider.RoleAssistant, am2.Role)
				require.Len(t, am2.Content, 1)
				assert.Equal(t, provider.ContentPartTypeText, am2.Content[0].Type)
				assert.Equal(t, "The temperature in NYC is 72F.", am2.Content[0].Text)
			},
		},
		{
			name: "parallel tool calls then text",
			parts: []Part{
				StepStartPart{},
				ToolInvocationPart{
					ToolCallID: "c1", ToolName: "weather", State: ToolStateOutputAvailable,
					Input: json.RawMessage(`{"city":"NYC"}`), Output: json.RawMessage(`{"temp":72}`),
				},
				ToolInvocationPart{
					ToolCallID: "c2", ToolName: "weather", State: ToolStateOutputAvailable,
					Input: json.RawMessage(`{"city":"LA"}`), Output: json.RawMessage(`{"temp":85}`),
				},
				StepStartPart{},
				TextPart{Text: "NYC is 72, LA is 85."},
			},
			check: func(t *testing.T, result []provider.Message) {
				require.Len(t, result, 3)
				assert.Len(t, result[0].Content, 2, "expected 2 tool calls")
				assert.Len(t, result[1].Content, 2, "expected 2 tool results")
				assert.Len(t, result[2].Content, 1, "expected 1 text part")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: tc.parts}}
			result := convert(t, msgs)
			tc.check(t, result)
		})
	}
}

func TestConvertToModelMessages_MultiRole(t *testing.T) {
	msgs := []UIMessage{
		{ID: "1", Role: RoleUser, Parts: []Part{TextPart{Text: "hello"}}},
		{ID: "2", Role: RoleAssistant, Parts: []Part{TextPart{Text: "hi"}}},
	}
	result := convert(t, msgs)
	require.Len(t, result, 2)
	assert.Equal(t, provider.RoleUser, result[0].Role)
	assert.Equal(t, provider.RoleAssistant, result[1].Role)
}

func TestToolSetToProviderTools(t *testing.T) {
	tests := []struct {
		name  string
		tools ToolSet
		check func(t *testing.T, pt []provider.Tool, warnings []provider.Warning)
	}{
		{
			name: "basic tool conversion",
			tools: ToolSet{"weather": Tool{
				Description: "Get weather",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
			}},
			check: func(t *testing.T, pt []provider.Tool, warnings []provider.Warning) {
				require.Len(t, pt, 1)
				assert.Equal(t, "weather", pt[0].Name)
				assert.Equal(t, provider.ToolTypeFunction, pt[0].Type)
				assert.Empty(t, warnings)
			},
		},
		{
			name: "forwards input examples and strict",
			tools: ToolSet{"calc": Tool{
				Description:   "Calculator",
				InputSchema:   testMustSchema(t, `{"type":"object"}`),
				InputExamples: []json.RawMessage{json.RawMessage(`{"x":1}`), json.RawMessage(`{"x":2}`)},
				Strict:        boolPtr(true),
			}},
			check: func(t *testing.T, pt []provider.Tool, warnings []provider.Warning) {
				require.Len(t, pt[0].InputExamples, 2)
				assert.JSONEq(t, `{"x":1}`, string(pt[0].InputExamples[0].Input))
				assert.JSONEq(t, `{"x":2}`, string(pt[0].InputExamples[1].Input))
				assert.Equal(t, boolPtr(true), pt[0].Strict)
				assert.Empty(t, warnings)
			},
		},
		{
			name:  "forwards explicit false strict",
			tools: ToolSet{"calc": Tool{Strict: boolPtr(false)}},
			check: func(t *testing.T, pt []provider.Tool, warnings []provider.Warning) {
				require.Len(t, pt, 1)
				assert.Equal(t, boolPtr(false), pt[0].Strict)
				assert.Empty(t, warnings)
			},
		},
		{
			name:  "leaves absent strict unset",
			tools: ToolSet{"calc": Tool{}},
			check: func(t *testing.T, pt []provider.Tool, warnings []provider.Warning) {
				require.Len(t, pt, 1)
				assert.Nil(t, pt[0].Strict)
				assert.Empty(t, warnings)
			},
		},
		{
			name:  "nil toolset returns nil",
			tools: nil,
			check: func(t *testing.T, pt []provider.Tool, warnings []provider.Warning) {
				assert.Nil(t, pt)
				assert.Nil(t, warnings)
			},
		},
		{
			name: "provider tool passes through ID Args and provider options",
			tools: ToolSet{"search": Tool{
				Type: UserToolProvider,
				ID:   "anthropic.web_search_20250305",
				Args: map[string]json.RawMessage{
					"maxUses": json.RawMessage(`5`),
				},
				ProviderOptions: provider.ProviderOptions{
					"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"cacheControl":{"type":"ephemeral"}}`)},
				},
			}},
			check: func(t *testing.T, pt []provider.Tool, warnings []provider.Warning) {
				require.Len(t, pt, 1)
				assert.Equal(t, provider.ToolTypeProvider, pt[0].Type)
				assert.Equal(t, "search", pt[0].Name)
				assert.Equal(t, "anthropic.web_search_20250305", pt[0].ID)
				assert.JSONEq(t, `5`, string(pt[0].Args["maxUses"]))
				data, err := json.Marshal(pt[0].ProviderOptions)
				require.NoError(t, err)
				assert.JSONEq(t, `{"anthropic":{"cacheControl":{"type":"ephemeral"}}}`, string(data))
				assert.Empty(t, warnings)
			},
		},
		{
			name: "empty Type defaults to function",
			tools: ToolSet{"weather": Tool{
				Description: "Get weather",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
			}},
			check: func(t *testing.T, pt []provider.Tool, warnings []provider.Warning) {
				require.Len(t, pt, 1)
				assert.Equal(t, provider.ToolTypeFunction, pt[0].Type)
				assert.Equal(t, "Get weather", pt[0].Description)
				assert.Empty(t, warnings)
			},
		},
		{
			name: "explicit function Type works",
			tools: ToolSet{"calc": Tool{
				Type:        UserToolFunction,
				Description: "Calculator",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
			}},
			check: func(t *testing.T, pt []provider.Tool, warnings []provider.Warning) {
				require.Len(t, pt, 1)
				assert.Equal(t, provider.ToolTypeFunction, pt[0].Type)
				assert.Equal(t, "Calculator", pt[0].Description)
				assert.Empty(t, warnings)
			},
		},
		{
			name: "mixed function and provider tools",
			tools: ToolSet{
				"weather": Tool{
					Description: "Get weather",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
				},
				"search": Tool{
					Type: UserToolProvider,
					ID:   "anthropic.web_search_20250305",
				},
			},
			check: func(t *testing.T, pt []provider.Tool, warnings []provider.Warning) {
				require.Len(t, pt, 2)
				assert.Equal(t, provider.ToolTypeProvider, pt[0].Type)
				assert.Equal(t, "search", pt[0].Name)
				assert.Equal(t, provider.ToolTypeFunction, pt[1].Type)
				assert.Equal(t, "weather", pt[1].Name)
				assert.Empty(t, warnings)
			},
		},
		{
			name: "dynamic Type treated as function",
			tools: ToolSet{"mcp_search": Tool{
				Type:        UserToolDynamic,
				Description: "MCP search",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
			}},
			check: func(t *testing.T, pt []provider.Tool, warnings []provider.Warning) {
				require.Len(t, pt, 1)
				assert.Equal(t, provider.ToolTypeFunction, pt[0].Type)
				assert.Equal(t, "mcp_search", pt[0].Name)
				assert.Equal(t, "MCP search", pt[0].Description)
				assert.Empty(t, warnings)
			},
		},
		{
			name: "unknown Type produces warning and skips tool",
			tools: ToolSet{
				"valid": Tool{
					Description: "Valid tool",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
				},
				"bad": Tool{
					Type:        "garbage",
					Description: "Bad tool",
				},
			},
			check: func(t *testing.T, pt []provider.Tool, warnings []provider.Warning) {
				require.Len(t, pt, 1, "invalid tool should be skipped")
				assert.Equal(t, "valid", pt[0].Name)
				require.Len(t, warnings, 1)
				assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
				assert.Contains(t, warnings[0].Feature, "bad")
				assert.Contains(t, warnings[0].Details, "garbage")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pt, warnings := toolSetToProviderTools(tc.tools)
			tc.check(t, pt, warnings)
		})
	}
}

func TestProviderMetadataToOptions(t *testing.T) {
	t.Run("wraps entries as RawProviderOption", func(t *testing.T) {
		meta := provider.ProviderMetadata{
			"anthropic": json.RawMessage(`{"cacheControl":{"type":"ephemeral"}}`),
			"openai":    json.RawMessage(`{"key":"val"}`),
		}
		opts := providerMetadataToOptions(meta)
		require.Len(t, opts, 2)

		anthOpt := opts["anthropic"]
		raw, ok := anthOpt.(provider.RawProviderOption)
		require.True(t, ok, "expected RawProviderOption, got %T", anthOpt)
		assert.Equal(t, "anthropic", raw.Key)
		assert.JSONEq(t, `{"cacheControl":{"type":"ephemeral"}}`, string(raw.Raw))

		oaiOpt := opts["openai"]
		raw2, ok := oaiOpt.(provider.RawProviderOption)
		require.True(t, ok, "expected RawProviderOption, got %T", oaiOpt)
		assert.Equal(t, "openai", raw2.Key)
		assert.JSONEq(t, `{"key":"val"}`, string(raw2.Raw))
	})

	t.Run("nil metadata returns nil", func(t *testing.T) {
		opts := providerMetadataToOptions(nil)
		assert.Nil(t, opts)
	})

	t.Run("empty metadata returns nil", func(t *testing.T) {
		opts := providerMetadataToOptions(provider.ProviderMetadata{})
		assert.Nil(t, opts)
	})

	t.Run("round-trip through ConvertToModelMessages", func(t *testing.T) {
		meta := provider.ProviderMetadata{
			"anthropic": json.RawMessage(`{"cacheControl":{"type":"ephemeral"}}`),
		}
		msgs := []UIMessage{{
			ID:   "1",
			Role: RoleUser,
			Parts: []Part{
				TextPart{Text: "hello", ProviderMetadata: meta},
			},
		}}
		result := convert(t, msgs)
		require.Len(t, result, 1)
		require.Len(t, result[0].Content, 1)
		cp := result[0].Content[0]

		raw, ok := cp.ProviderOptions["anthropic"].(provider.RawProviderOption)
		require.True(t, ok, "expected RawProviderOption, got %T", cp.ProviderOptions["anthropic"])
		assert.Equal(t, "anthropic", raw.Key)
		assert.JSONEq(t, `{"cacheControl":{"type":"ephemeral"}}`, string(raw.Raw))
	})
}
