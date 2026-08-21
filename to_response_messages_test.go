package aisdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/internal/providerrequest"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestToResponseMessages exercises the public helper end-to-end. The cases
// are ported from upstream Vercel AI SDK
// packages/ai/src/generate-text/to-response-messages.test.ts. Cases that
// depend on upstream-only ContentPart variants (`tool-error` as a separate
// kind, `tool-approval-request` as an input variant) are covered indirectly
// through ToolResultOutput.Type and the existing approval-response routing.
func TestToResponseMessages(t *testing.T) {
	t.Run("empty input produces empty result", func(t *testing.T) {
		got := ToResponseMessages(nil)
		assert.Empty(t, got)
	})

	t.Run("text-only input produces a single assistant message", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{provider.TextPart("Hello, world!")},
		)
		require.Len(t, got, 1)
		assert.Equal(t, provider.RoleAssistant, got[0].Role)
		require.Len(t, got[0].Content, 1)
		assert.Equal(t, provider.ContentPartTypeText, got[0].Content[0].Type)
		assert.Equal(t, "Hello, world!", got[0].Content[0].Text)
	})

	t.Run("empty text part is dropped", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{
				provider.TextPart(""),
				provider.ToolCallPart("123", "testTool", json.RawMessage(`{}`)),
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 1, "empty text dropped, only tool-call remains")
		assert.Equal(t, provider.ContentPartTypeToolCall, got[0].Content[0].Type)
	})

	t.Run("text + tool-call assembled in order", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{
				provider.TextPart("Using a tool"),
				provider.ToolCallPart("123", "testTool", json.RawMessage(`{}`)),
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 2)
		assert.Equal(t, provider.ContentPartTypeText, got[0].Content[0].Type)
		assert.Equal(t, provider.ContentPartTypeToolCall, got[0].Content[1].Type)
		assert.Equal(t, "123", got[0].Content[1].ToolCallID)
		assert.Equal(t, "testTool", got[0].Content[1].ToolName)
	})

	t.Run("tool-call ProviderOptions and ProviderExecuted carry through", func(t *testing.T) {
		opts := provider.ProviderOptions{
			"testProvider": provider.RawProviderOption{Key: "testProvider", Raw: json.RawMessage(`{"signature":"sig"}`)},
		}
		got := ToResponseMessages(
			[]provider.ContentPart{
				provider.TextPart("Using a tool"),
				{
					Type:             provider.ContentPartTypeToolCall,
					ToolCallID:       "123",
					ToolName:         "testTool",
					Input:            json.RawMessage(`{}`),
					ProviderExecuted: boolPtr(false),
					ProviderOptions:  opts,
				},
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 2)
		assert.Equal(t, opts, got[0].Content[1].ProviderOptions)
	})

	t.Run("tool-result routed to a tool message", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{
				provider.TextPart("Tool used"),
				provider.ToolCallPart("123", "testTool", json.RawMessage(`{}`)),
				{
					Type:       provider.ContentPartTypeToolResult,
					ToolCallID: "123",
					ToolName:   "testTool",
					Output: &provider.ToolResultOutput{
						Type: provider.ToolOutputText,
						Text: "Tool result",
					},
				},
			},
		)
		require.Len(t, got, 2)

		require.Equal(t, provider.RoleAssistant, got[0].Role)
		require.Len(t, got[0].Content, 2, "text + tool-call")

		require.Equal(t, provider.RoleTool, got[1].Role)
		require.Len(t, got[1].Content, 1)
		assert.Equal(t, provider.ContentPartTypeToolResult, got[1].Content[0].Type)
		assert.Equal(t, "Tool result", got[1].Content[0].Output.Text)
	})

	t.Run("tool-error encoded as ToolOutputErrorText routed to tool message", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{
				provider.TextPart("Tool used"),
				provider.ToolCallPart("123", "testTool", json.RawMessage(`{}`)),
				{
					Type:       provider.ContentPartTypeToolResult,
					ToolCallID: "123",
					ToolName:   "testTool",
					Output: &provider.ToolResultOutput{
						Type: provider.ToolOutputErrorText,
						Text: "Tool error",
					},
				},
			},
		)
		require.Len(t, got, 2)
		require.Len(t, got[1].Content, 1)
		assert.Equal(t, provider.ToolOutputErrorText, got[1].Content[0].Output.Type)
		assert.Equal(t, "Tool error", got[1].Content[0].Output.Text)
	})

	t.Run("parallel tool results follow tool call order", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{
				provider.TextPart("Using tools"),
				provider.ToolCallPart("call-a", "toolA", json.RawMessage(`{}`)),
				provider.ToolCallPart("call-b", "toolB", json.RawMessage(`{}`)),
				{
					Type:       provider.ContentPartTypeToolResult,
					ToolCallID: "call-b",
					ToolName:   "toolB",
					Output:     &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "B result"},
				},
				{
					Type:       provider.ContentPartTypeToolResult,
					ToolCallID: "call-a",
					ToolName:   "toolA",
					Output:     &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "A result"},
				},
			},
		)
		require.Len(t, got, 2)
		require.Len(t, got[1].Content, 2)
		assert.Equal(t, []string{"call-a", "call-b"}, []string{
			got[1].Content[0].ToolCallID,
			got[1].Content[1].ToolCallID,
		})
	})

	t.Run("reasoning with provider signature is preserved", func(t *testing.T) {
		opts := provider.ProviderOptions{
			"testProvider": provider.RawProviderOption{Key: "testProvider", Raw: json.RawMessage(`{"signature":"sig"}`)},
		}
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:            provider.ContentPartTypeReasoning,
					Text:            "Thinking text",
					ProviderOptions: opts,
				},
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 1)
		assert.Equal(t, provider.ContentPartTypeReasoning, got[0].Content[0].Type)
		assert.Equal(t, "Thinking text", got[0].Content[0].Text)
		assert.Equal(t, opts, got[0].Content[0].ProviderOptions, "ProviderOptions must survive (the #171 fix)")
	})

	t.Run("multiple reasoning blocks preserve order with text", func(t *testing.T) {
		redactedOpts := provider.ProviderOptions{
			"testProvider": provider.RawProviderOption{Key: "testProvider", Raw: json.RawMessage(`{"isRedacted":true}`)},
		}
		signedOpts := provider.ProviderOptions{
			"testProvider": provider.RawProviderOption{Key: "testProvider", Raw: json.RawMessage(`{"signature":"sig"}`)},
		}
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:            provider.ContentPartTypeReasoning,
					Text:            "redacted-data",
					ProviderOptions: redactedOpts,
				},
				{
					Type:            provider.ContentPartTypeReasoning,
					Text:            "Thinking text",
					ProviderOptions: signedOpts,
				},
				provider.TextPart("Final text"),
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 3)
		assert.Equal(t, "redacted-data", got[0].Content[0].Text)
		assert.Equal(t, redactedOpts, got[0].Content[0].ProviderOptions)
		assert.Equal(t, "Thinking text", got[0].Content[1].Text)
		assert.Equal(t, signedOpts, got[0].Content[1].ProviderOptions)
		assert.Equal(t, "Final text", got[0].Content[2].Text)
	})

	t.Run("custom parts pass through", func(t *testing.T) {
		opts := provider.ProviderOptions{
			"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"itemId":"cmp_123"}`)},
		}
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:            provider.ContentPartTypeCustom,
					Kind:            "mock-provider.compaction",
					ProviderOptions: opts,
				},
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 1)
		assert.Equal(t, provider.ContentPartTypeCustom, got[0].Content[0].Type)
		assert.Equal(t, "mock-provider.compaction", got[0].Content[0].Kind)
		assert.Equal(t, opts, got[0].Content[0].ProviderOptions)
	})

	t.Run("file part appended to assistant message", func(t *testing.T) {
		data := &provider.DataContent{Base64: "iVBORw0KGgo="}
		got := ToResponseMessages(
			[]provider.ContentPart{
				provider.TextPart("Here is an image"),
				{
					Type:      provider.ContentPartTypeFile,
					Data:      data,
					MediaType: "image/png",
				},
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 2)
		assert.Equal(t, provider.ContentPartTypeFile, got[0].Content[1].Type)
		assert.Equal(t, "image/png", got[0].Content[1].MediaType)
		require.NotNil(t, got[0].Content[1].Data)
		assert.Equal(t, "iVBORw0KGgo=", got[0].Content[1].Data.Base64)
	})

	t.Run("generated file filename moves to request ownership", func(t *testing.T) {
		data := provider.TextDataContent("value")
		got := ToResponseMessages([]provider.ContentPart{{
			Type: provider.ContentPartTypeFile, Data: &data, MediaType: "text/plain", Filename: "report.txt",
		}})
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 1)
		file := got[0].Content[0]
		require.NotNil(t, file.FilePartFilename)
		assert.Equal(t, "report.txt", *file.FilePartFilename)
		assert.Empty(t, file.Filename)
		require.NoError(t, providerrequest.Validate(provider.CallOptions{Prompt: got}))
	})

	t.Run("request filename presence is copied defensively", func(t *testing.T) {
		data := provider.TextDataContent("value")
		empty := ""
		got := ToResponseMessages([]provider.ContentPart{{
			Type: provider.ContentPartTypeFile, Data: &data, MediaType: "text/plain",
			FilePartFilename: &empty, Filename: "invalid-generated.txt",
		}})
		require.Len(t, got, 1)
		file := got[0].Content[0]
		require.NotNil(t, file.FilePartFilename)
		assert.Empty(t, *file.FilePartFilename)
		assert.Empty(t, file.Filename)
		assert.NotSame(t, &empty, file.FilePartFilename)
	})

	t.Run("reasoning-file part preserves Data, MediaType, and ProviderOptions", func(t *testing.T) {
		opts := provider.ProviderOptions{
			"testProvider": provider.RawProviderOption{Key: "testProvider", Raw: json.RawMessage(`{"signature":"sig"}`)},
		}
		data := &provider.DataContent{Base64: "iVBORw0KGgo="}
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:            provider.ContentPartTypeReasoningFile,
					Data:            data,
					MediaType:       "image/png",
					ProviderOptions: opts,
				},
				provider.TextPart("Here is my analysis"),
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 2)
		assert.Equal(t, provider.ContentPartTypeReasoningFile, got[0].Content[0].Type)
		assert.Equal(t, "image/png", got[0].Content[0].MediaType)
		assert.Equal(t, opts, got[0].Content[0].ProviderOptions)
	})

	t.Run("reasoning + file + text + tool-call ordering preserved", func(t *testing.T) {
		opts := provider.ProviderOptions{
			"testProvider": provider.RawProviderOption{Key: "testProvider", Raw: json.RawMessage(`{"signature":"sig"}`)},
		}
		data := &provider.DataContent{Base64: "iVBORw0KGgo="}
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:            provider.ContentPartTypeReasoning,
					Text:            "Thinking text",
					ProviderOptions: opts,
				},
				{Type: provider.ContentPartTypeFile, Data: data, MediaType: "image/png"},
				provider.TextPart("Combined response"),
				provider.ToolCallPart("123", "testTool", json.RawMessage(`{}`)),
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 4)
		assert.Equal(t, provider.ContentPartTypeReasoning, got[0].Content[0].Type)
		assert.Equal(t, provider.ContentPartTypeFile, got[0].Content[1].Type)
		assert.Equal(t, provider.ContentPartTypeText, got[0].Content[2].Type)
		assert.Equal(t, provider.ContentPartTypeToolCall, got[0].Content[3].Type)
	})

	t.Run("provider-executed tool call + result inlined in assistant message", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{
				provider.TextPart("Let me search."),
				{
					Type:             provider.ContentPartTypeToolCall,
					ToolCallID:       "srvtoolu_1",
					ToolName:         "web_search",
					Input:            json.RawMessage(`{"query":"test"}`),
					ProviderExecuted: boolPtr(true),
				},
				{
					Type:             provider.ContentPartTypeToolResult,
					ToolCallID:       "srvtoolu_1",
					ToolName:         "web_search",
					Output:           &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`[{"url":"https://example.com"}]`)},
					ProviderExecuted: boolPtr(true),
				},
				provider.TextPart("Done."),
			},
		)
		require.Len(t, got, 1, "no separate tool message for provider-executed-only step")
		require.Len(t, got[0].Content, 4)
		assert.Equal(t, provider.ContentPartTypeText, got[0].Content[0].Type)
		assert.Equal(t, provider.ContentPartTypeToolCall, got[0].Content[1].Type)
		assert.Equal(t, provider.ContentPartTypeToolResult, got[0].Content[2].Type)
		assert.Nil(t, got[0].Content[2].ProviderExecuted)
		assert.Equal(t, provider.ContentPartTypeText, got[0].Content[3].Type)
	})

	t.Run("tool-approval-request preserves signed metadata", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{
				provider.ToolCallPart("call-1", "weather", json.RawMessage(`{"city":"Tokyo"}`)),
				{
					Type:        provider.ContentPartTypeToolApprovalRequest,
					ApprovalID:  "approval-1",
					ToolCallID:  "call-1",
					ToolName:    "weather",
					Signature:   "signed-approval-envelope",
					IsAutomatic: true,
				},
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 2)
		approval := got[0].Content[1]
		assert.Equal(t, provider.ContentPartTypeToolApprovalRequest, approval.Type)
		assert.Equal(t, "approval-1", approval.ApprovalID)
		assert.Equal(t, "call-1", approval.ToolCallID)
		assert.Equal(t, "weather", approval.ToolName)
		assert.Equal(t, "signed-approval-envelope", approval.Signature)
		assert.True(t, approval.IsAutomatic)
	})

	t.Run("mixed provider-executed inline + non-provider-executed in tool message", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:             provider.ContentPartTypeToolCall,
					ToolCallID:       "srv-1",
					ToolName:         "web_search",
					Input:            json.RawMessage(`{}`),
					ProviderExecuted: boolPtr(true),
				},
				{
					Type:             provider.ContentPartTypeToolResult,
					ToolCallID:       "srv-1",
					ToolName:         "web_search",
					Output:           &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`[]`)},
					ProviderExecuted: boolPtr(true),
				},
				provider.ToolCallPart("tc-1", "report", json.RawMessage(`{}`)),
				{
					Type:       provider.ContentPartTypeToolResult,
					ToolCallID: "tc-1",
					ToolName:   "report",
					Output:     &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"ok":true}`)},
				},
			},
		)
		require.Len(t, got, 2)
		am := got[0]
		require.Len(t, am.Content, 3, "two calls + one inline result")
		assert.Equal(t, provider.ContentPartTypeToolResult, am.Content[1].Type, "provider-executed result inlined after its call")
		assert.Equal(t, provider.ContentPartTypeToolCall, am.Content[2].Type)
		assert.Equal(t, "tc-1", am.Content[2].ToolCallID)

		tm := got[1]
		require.Len(t, tm.Content, 1, "only the non-provider-executed result")
		assert.Equal(t, "tc-1", tm.Content[0].ToolCallID)
	})

	t.Run("provider-executed result preserves input order with interleaved text", func(t *testing.T) {
		// Regression test for upstream-parity drift: provider-executed
		// tool-results MUST appear at their input position rather than
		// being inlined immediately after the matching tool-call. Upstream
		// processes provider-executed tool-results in its main content
		// loop; pre-indexing + inline-after-call rewrites the order
		// observed by public callers.
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:             provider.ContentPartTypeToolCall,
					ToolCallID:       "srv-1",
					ToolName:         "web_search",
					Input:            json.RawMessage(`{}`),
					ProviderExecuted: boolPtr(true),
				},
				provider.TextPart("interleaved note"),
				{
					Type:             provider.ContentPartTypeToolResult,
					ToolCallID:       "srv-1",
					ToolName:         "web_search",
					Output:           &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`[]`)},
					ProviderExecuted: boolPtr(true),
				},
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 3)
		assert.Equal(t, provider.ContentPartTypeToolCall, got[0].Content[0].Type)
		assert.Equal(t, provider.ContentPartTypeText, got[0].Content[1].Type)
		assert.Equal(t, "interleaved note", got[0].Content[1].Text)
		assert.Equal(t, provider.ContentPartTypeToolResult, got[0].Content[2].Type)
		assert.Equal(t, "srv-1", got[0].Content[2].ToolCallID)
	})

	t.Run("provider-executed-only step produces no tool message", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:             provider.ContentPartTypeToolCall,
					ToolCallID:       "srv-1",
					ToolName:         "ws",
					Input:            json.RawMessage(`{}`),
					ProviderExecuted: boolPtr(true),
				},
				{
					Type:             provider.ContentPartTypeToolResult,
					ToolCallID:       "srv-1",
					ToolName:         "ws",
					Output:           &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{}`)},
					ProviderExecuted: boolPtr(true),
				},
			},
		)
		require.Len(t, got, 1, "only assistant message")
		assert.Equal(t, provider.RoleAssistant, got[0].Role)
	})

	t.Run("tool-call with valid primitive input is preserved", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:       provider.ContentPartTypeToolCall,
					ToolCallID: "call-1",
					ToolName:   "weather",
					Input:      json.RawMessage(`42`),
				},
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 1)
		assert.Equal(t, provider.ContentPartTypeToolCall, got[0].Content[0].Type)
		assert.JSONEq(t, `42`, string(got[0].Content[0].Input))
	})

	t.Run("tool-call with valid object input is preserved verbatim", func(t *testing.T) {
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:       provider.ContentPartTypeToolCall,
					ToolCallID: "call-1",
					ToolName:   "weather",
					Input:      json.RawMessage(`{"cities":"San Francisco"}`),
				},
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 1)
		assert.JSONEq(t, `{"cities":"San Francisco"}`, string(got[0].Content[0].Input))
	})

	t.Run("text part ProviderOptions preserved", func(t *testing.T) {
		opts := provider.ProviderOptions{
			"testProvider": provider.RawProviderOption{Key: "testProvider", Raw: json.RawMessage(`{"signature":"sig"}`)},
		}
		got := ToResponseMessages(
			[]provider.ContentPart{
				{Type: provider.ContentPartTypeText, Text: "Here is a text", ProviderOptions: opts},
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 1)
		assert.Equal(t, opts, got[0].Content[0].ProviderOptions)
	})

	t.Run("tool-result ProviderOptions carry through", func(t *testing.T) {
		opts := provider.ProviderOptions{
			"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"cacheControl":"x"}`)},
		}
		got := ToResponseMessages(
			[]provider.ContentPart{
				provider.ToolCallPart("tc-1", "t", json.RawMessage(`{}`)),
				{
					Type:            provider.ContentPartTypeToolResult,
					ToolCallID:      "tc-1",
					ToolName:        "t",
					Output:          &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"},
					ProviderOptions: opts,
				},
			},
		)
		require.Len(t, got, 2)
		require.Len(t, got[1].Content, 1)
		assert.Equal(t, opts, got[1].Content[0].ProviderOptions)
	})

	t.Run("denied tool-approval-response adds an execution-denied tool result", func(t *testing.T) {
		approved := false
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:       provider.ContentPartTypeToolApprovalResponse,
					ApprovalID: "approval-1",
					ToolCallID: "tc-1",
					ToolName:   "weather",
					Approved:   &approved,
					Reason:     requestStringPointer("user denied"),
				},
			},
		)
		// No assistant message (no assistant content), only the tool message.
		require.Len(t, got, 1)
		assert.Equal(t, provider.RoleTool, got[0].Role)
		require.Len(t, got[0].Content, 2, "approval response + synthetic execution-denied result")

		approvalPart := got[0].Content[0]
		assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, approvalPart.Type)
		assert.Equal(t, "approval-1", approvalPart.ApprovalID)
		assert.Equal(t, "tc-1", approvalPart.ToolCallID)
		assert.Equal(t, "weather", approvalPart.ToolName)

		// The synthetic execution-denied tool-result must carry the tool
		// call ID and name from the approval so the model can correlate
		// it with the original tool call (matches upstream which reads
		// part.toolCall.{toolCallId,toolName}).
		denied := got[0].Content[1]
		assert.Equal(t, provider.ContentPartTypeToolResult, denied.Type)
		assert.Equal(t, "tc-1", denied.ToolCallID)
		assert.Equal(t, "weather", denied.ToolName)
		require.NotNil(t, denied.Output)
		assert.Equal(t, provider.ToolOutputExecutionDenied, denied.Output.Type)
		require.NotNil(t, denied.Output.Reason)
		assert.Equal(t, "user denied", *denied.Output.Reason)
	})

	t.Run("approved tool-approval-response routes without synthetic result", func(t *testing.T) {
		approved := true
		got := ToResponseMessages(
			[]provider.ContentPart{
				{
					Type:             provider.ContentPartTypeToolApprovalResponse,
					ApprovalID:       "approval-1",
					Approved:         &approved,
					ProviderExecuted: boolPtr(true),
				},
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 1)
		assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, got[0].Content[0].Type)
	})

	t.Run("source content (modeled outside provider.ContentPart) is not part of the wire", func(t *testing.T) {
		// Sources don't have a provider.ContentPart variant in the Go port,
		// matching upstream's "skip sources" behavior naturally.
		got := ToResponseMessages(
			[]provider.ContentPart{
				provider.TextPart("text"),
			},
		)
		require.Len(t, got, 1)
		require.Len(t, got[0].Content, 1)
		assert.Equal(t, provider.ContentPartTypeText, got[0].Content[0].Type)
	})
}

// TestStreamTextResponseMessages drives a small two-step StreamText run and
// asserts that result.Response().Messages reflects the last step and
// matches result.Steps()[len-1].Response.Messages — the surfacing
// requirement from spec to-response-messages.
func TestStreamTextResponseMessages(t *testing.T) {
	callNum := 0
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			callNum++
			if callNum == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("weather", `{"city":"NYC"}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("It's 72F in NYC")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("weather?")),
		WithTools(ToolSet{
			"weather": Tool{
				Description: "Get weather",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{"temp":72}`), nil
				},
			},
		}),
		WithStopWhen(StepCountIs(5)),
	)

	for range result.FullStream() {
	}

	steps := result.Steps()
	require.Len(t, steps, 2)

	// Step 1: tool-call only (no text). The Response.Messages SHALL contain
	// an assistant message (with the tool-call) and a tool message (with the
	// tool-result).
	require.NotEmpty(t, steps[0].Response.Messages, "step 0 Response.Messages must be populated")
	require.Len(t, steps[0].Response.Messages, 2, "assistant + tool")
	assert.Equal(t, provider.RoleAssistant, steps[0].Response.Messages[0].Role)
	assert.Equal(t, provider.RoleTool, steps[0].Response.Messages[1].Role)

	// Step 2: text-only finish. Response.Messages SHALL contain a single
	// assistant message.
	require.NotEmpty(t, steps[1].Response.Messages, "step 1 Response.Messages must be populated")
	require.Len(t, steps[1].Response.Messages, 1)
	assert.Equal(t, provider.RoleAssistant, steps[1].Response.Messages[0].Role)
	require.Len(t, steps[1].Response.Messages[0].Content, 1)
	assert.Equal(t, "It's 72F in NYC", steps[1].Response.Messages[0].Content[0].Text)

	// result.Response() reflects the last step.
	assert.Equal(t, steps[1].Response.Messages, result.Response().Messages)
}

// TestStreamTextResponseMessages_GeneratedFiles asserts that file parts
// captured into step.Files during a StreamText run are propagated to the
// assistant message in step.Response.Messages. This is the regression
// guard for the gap closed in buildResponseContent: previously files were
// kept on step.Files but dropped from response.messages, so they were
// missing from the next-call context for multi-step generations.
func TestStreamTextResponseMessages_GeneratedFiles(t *testing.T) {
	pngBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	jpegBytes := []byte{0xFF, 0xD8, 0xFF}
	fileMetadata := provider.ProviderMetadata{"test": json.RawMessage(`{"kind":"file"}`)}
	textMetadata := provider.ProviderMetadata{"test": json.RawMessage(`{"kind":"text-end"}`)}
	reasoningMetadata := provider.ProviderMetadata{"test": json.RawMessage(`{"kind":"reasoning"}`)}

	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 10)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: pngBytes}, MediaType: "image/png", ProviderMetadata: fileMetadata}
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
				ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "Here are the images"}
				ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1", ProviderMetadata: textMetadata}
				ch <- provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: jpegBytes}, MediaType: "image/jpeg"}
				ch <- provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "https://example.com/image.png"}, MediaType: "image/png"}
				ch <- provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData}, MediaType: "application/octet-stream"}
				ch <- provider.StreamPart{Type: provider.PartReasoningStart, ID: "r1", ProviderMetadata: reasoningMetadata}
				ch <- provider.StreamPart{Type: provider.PartReasoningDelta, ID: "r1", Delta: "thinking", ProviderMetadata: reasoningMetadata}
				ch <- provider.StreamPart{Type: provider.PartReasoningFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "https://example.com/reasoning.png"}, MediaType: "image/png", ProviderMetadata: reasoningMetadata}
				ch <- provider.StreamPart{Type: provider.PartReasoningEnd, ID: "r1", ProviderMetadata: reasoningMetadata}
				ch <- provider.StreamPart{
					Type:         provider.PartFinish,
					FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
					Usage:        &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(3)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(10)}},
				}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("generate images")),
	)

	var streamedReasoningFiles []StreamReasoningFile
	for part := range result.FullStream() {
		if file, ok := part.(StreamReasoningFile); ok {
			streamedReasoningFiles = append(streamedReasoningFiles, file)
		}
	}

	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].Files, 4, "all files captured on step.Files")

	// Response.Messages SHALL include both files in the assistant message.
	require.Len(t, steps[0].Response.Messages, 1, "single assistant message")
	assistant := steps[0].Response.Messages[0]
	assert.Equal(t, provider.RoleAssistant, assistant.Role)

	// Streamed content keeps its provider order: file, text, three files,
	// reasoning text, then the reasoning file.
	require.Len(t, assistant.Content, 7)
	assert.Equal(t, provider.ContentPartTypeFile, assistant.Content[0].Type)
	require.NotNil(t, assistant.Content[0].Data)
	assert.Equal(t, pngBytes, assistant.Content[0].Data.Bytes)

	assert.Equal(t, provider.ContentPartTypeText, assistant.Content[1].Type)
	assert.Equal(t, "Here are the images", assistant.Content[1].Text)
	assert.Equal(t, providerMetadataToOptions(textMetadata), assistant.Content[1].ProviderOptions)

	assert.Equal(t, provider.ContentPartTypeFile, assistant.Content[2].Type)
	assert.Equal(t, "image/jpeg", assistant.Content[2].MediaType)
	require.NotNil(t, assistant.Content[2].Data)
	assert.Equal(t, jpegBytes, assistant.Content[2].Data.Bytes)

	assert.Equal(t, provider.ContentPartTypeFile, assistant.Content[3].Type)
	require.NotNil(t, assistant.Content[3].Data)
	assert.Equal(t, "https://example.com/image.png", assistant.Content[3].Data.Base64)
	assert.Equal(t, "https://example.com/image.png", steps[0].Files[2].Base64)
	assert.Equal(t, "data:image/png;base64,https://example.com/image.png", steps[0].Files[2].DataURL())

	assert.Equal(t, provider.ContentPartTypeFile, assistant.Content[4].Type)
	require.NotNil(t, assistant.Content[4].Data)
	assert.NotNil(t, assistant.Content[4].Data.Bytes)
	assert.Empty(t, assistant.Content[4].Data.Bytes)
	emptyDataJSON, err := json.Marshal(assistant.Content[4].Data)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"data","data":""}`, string(emptyDataJSON))

	assert.Equal(t, provider.ContentPartTypeReasoning, assistant.Content[5].Type)
	assert.Equal(t, "thinking", assistant.Content[5].Text)

	assert.Equal(t, provider.ContentPartTypeReasoningFile, assistant.Content[6].Type)
	require.NotNil(t, assistant.Content[6].Data)
	assert.Equal(t, "https://example.com/reasoning.png", assistant.Content[6].Data.Base64)
	require.Len(t, streamedReasoningFiles, 1)
	assert.Equal(t, "https://example.com/reasoning.png", streamedReasoningFiles[0].File.Base64)
	assert.Equal(t, reasoningMetadata, streamedReasoningFiles[0].ProviderMetadata)
	chunks := translateToChunks(streamedReasoningFiles[0], uiMessageStreamConfig{})
	require.Len(t, chunks, 1)
	assert.Equal(t, ChunkReasoningFile, chunks[0].Type)
	// The registered upstream ai baseline stringifies URL-valued generated files
	// and uses that string as base64 when constructing the UI data URL.
	assert.Equal(t, "data:image/png;base64,https://example.com/reasoning.png", chunks[0].URL)
	assert.Equal(t, reasoningMetadata, chunks[0].ProviderMetadata)
	chunkJSON, err := json.Marshal(chunks[0])
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"reasoning-file","url":"data:image/png;base64,https://example.com/reasoning.png","mediaType":"image/png","providerMetadata":{"test":{"kind":"reasoning"}}}`, string(chunkJSON))

	require.Len(t, steps[0].Reasoning, 2)
	reasoningText, ok := steps[0].Reasoning[0].(ReasoningTextOutput)
	require.True(t, ok)
	assert.Equal(t, "thinking", reasoningText.Text)
	reasoningOutput, ok := steps[0].Reasoning[1].(ReasoningFileOutput)
	require.True(t, ok)
	assert.Equal(t, "https://example.com/reasoning.png", reasoningOutput.File.Base64)
	assert.Equal(t, reasoningMetadata, reasoningOutput.ProviderMetadata)

	require.Len(t, steps[0].Content, 7)
	firstFile, ok := steps[0].Content[0].(FileContent)
	require.True(t, ok)
	assert.Equal(t, pngBytes, firstFile.File.Data)
	assert.Equal(t, fileMetadata, firstFile.ProviderMetadata)
	text, ok := steps[0].Content[1].(TextContent)
	require.True(t, ok)
	assert.Equal(t, textMetadata, text.ProviderMetadata)
	_, ok = steps[0].Content[2].(FileContent)
	assert.True(t, ok)
	_, ok = steps[0].Content[3].(FileContent)
	assert.True(t, ok)
	_, ok = steps[0].Content[4].(FileContent)
	assert.True(t, ok)
	reasoningTextContent, ok := steps[0].Content[5].(ReasoningContent)
	require.True(t, ok)
	assert.Equal(t, "thinking", reasoningTextContent.Text)
	reasoningContent, ok := steps[0].Content[6].(ReasoningFileContent)
	require.True(t, ok)
	assert.Equal(t, "https://example.com/reasoning.png", reasoningContent.File.Base64)
	assert.Equal(t, reasoningMetadata, reasoningContent.ProviderMetadata)

	// result.Response() reflects the last step.
	assert.Equal(t, steps[0].Response.Messages, result.Response().Messages)

	streamReasoning := result.Reasoning()
	require.Len(t, streamReasoning, 2)
	_, ok = streamReasoning[0].(ReasoningTextOutput)
	assert.True(t, ok)
	_, ok = streamReasoning[1].(ReasoningFileOutput)
	assert.True(t, ok)

	generated, err := GenerateText(context.Background(), model,
		WithModelMessages(provider.UserText("generate images")),
	)
	require.NoError(t, err)
	require.Len(t, generated.Reasoning, 2)
	_, ok = generated.Reasoning[0].(ReasoningTextOutput)
	assert.True(t, ok)
	_, ok = generated.Reasoning[1].(ReasoningFileOutput)
	assert.True(t, ok)
}
