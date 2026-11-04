package aisdk

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repeatedApprovalMessages(responses ...provider.ContentPart) []provider.Message {
	return []provider.Message{
		provider.NewAssistantMessage(
			provider.ToolCallPart("call", "save", json.RawMessage(`{}`)),
			provider.ToolApprovalRequestPart("approval", "call", false),
			provider.ToolApprovalRequestPart("another-approval", "call", false),
		),
		provider.NewToolMessage(responses...),
	}
}

func TestCollectToolApprovals_RepeatedResponses(t *testing.T) {
	for _, approved := range []bool{true, false} {
		first := provider.ToolApprovalResponsePart("approval", approved, "first reason")
		for _, tc := range []struct {
			name   string
			replay provider.ContentPart
		}{
			{"identical", first},
			{"different reason", provider.ToolApprovalResponsePart("approval", approved, "later reason")},
			{"different metadata", provider.ProviderExecutedToolApprovalResponsePart("approval", approved, "first reason")},
			{"conflicting", provider.ToolApprovalResponsePart("approval", !approved, "later reason")},
			{"another approval id", provider.ToolApprovalResponsePart("another-approval", approved, "later reason")},
		} {
			t.Run(fmt.Sprintf("approved=%t/%s", approved, tc.name), func(t *testing.T) {
				accepted, denied, err := collectToolApprovals(repeatedApprovalMessages(first, tc.replay))
				require.NoError(t, err)
				if approved {
					require.Empty(t, denied)
					require.Len(t, accepted, 1)
					assert.Equal(t, first, accepted[0].response)
				} else {
					require.Empty(t, accepted)
					require.Len(t, denied, 1)
					assert.Equal(t, first, denied[0].response)
				}
			})
		}
	}
}

func TestCollectToolApprovals_ReplayBoundaries(t *testing.T) {
	approved := provider.ToolApprovalResponsePart("approval", true, "")
	denied := provider.ToolApprovalResponsePart("approval", false, "denied")
	result := provider.ToolResultPart("call", "save", &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied})
	t.Run("distinct calls are collected independently", func(t *testing.T) {
		other := provider.ToolApprovalResponsePart("other-approval", true, "")
		msgs := repeatedApprovalMessages(approved, other, denied, other)
		msgs[0].Content = append(msgs[0].Content,
			provider.ToolCallPart("other-call", "save", json.RawMessage(`{}`)),
			provider.ToolApprovalRequestPart("other-approval", "other-call", false),
		)
		accepted, rejected, err := collectToolApprovals(msgs)
		require.NoError(t, err)
		assert.Empty(t, rejected)
		require.Len(t, accepted, 2)
		assert.Equal(t, "call", accepted[0].toolCall.ToolCallID)
		assert.Equal(t, "other-call", accepted[1].toolCall.ToolCallID)
	})
	t.Run("existing denial skips approval before collecting denial once", func(t *testing.T) {
		accepted, rejected, err := collectToolApprovals(repeatedApprovalMessages(approved, denied, denied, result))
		require.NoError(t, err)
		assert.Empty(t, accepted)
		require.Len(t, rejected, 1)
		assert.Equal(t, denied, rejected[0].response)
		assert.True(t, rejected[0].hasExistingToolResult)
	})
	t.Run("completed result suppresses all responses", func(t *testing.T) {
		completed := provider.ToolResultPart("call", "save", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "saved"})
		accepted, rejected, err := collectToolApprovals(repeatedApprovalMessages(approved, denied, completed))
		require.NoError(t, err)
		assert.Empty(t, accepted)
		assert.Empty(t, rejected)
	})
	t.Run("earlier result does not suppress last message", func(t *testing.T) {
		msgs := repeatedApprovalMessages(result)
		msgs = append(msgs, provider.NewToolMessage(approved, approved))
		accepted, rejected, err := collectToolApprovals(msgs)
		require.NoError(t, err)
		assert.Empty(t, rejected)
		require.Len(t, accepted, 1)
		assert.False(t, accepted[0].hasExistingToolResult)
	})
	t.Run("non-tool last message is not collected", func(t *testing.T) {
		msgs := append(repeatedApprovalMessages(approved, approved), provider.UserText("continue"))
		accepted, rejected, err := collectToolApprovals(msgs)
		require.NoError(t, err)
		assert.Empty(t, accepted)
		assert.Empty(t, rejected)
	})
	for _, tc := range []struct {
		name     string
		response provider.ContentPart
	}{
		{"missing approved", provider.ContentPart{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval"}},
		{"unknown approval", provider.ToolApprovalResponsePart("unknown", true, "")},
		{"missing tool call", provider.ToolApprovalResponsePart("orphan", true, "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msgs := repeatedApprovalMessages(approved, tc.response)
			msgs[0].Content = append(msgs[0].Content, provider.ToolApprovalRequestPart("orphan", "missing", false))
			_, _, err := collectToolApprovals(msgs)
			if tc.name == "missing tool call" {
				var missing *ToolCallNotFoundForApprovalError
				require.ErrorAs(t, err, &missing)
			} else {
				var invalid *InvalidToolApprovalError
				require.ErrorAs(t, err, &invalid)
			}
		})
	}
}

func TestToolApproval_RepeatedResponseContinuation(t *testing.T) {
	for _, api := range []string{"stream", "generate", "agent stream", "agent generate"} {
		for _, approved := range []bool{true, false} {
			for _, replay := range []string{"identical", "conflicting", "another approval id", "existing denial", "converted UI"} {
				if approved && replay == "existing denial" {
					continue
				}
				t.Run(fmt.Sprintf("%s/approved=%t/%s", api, approved, replay), func(t *testing.T) {
					first := provider.ToolApprovalResponsePart("approval", approved, "first reason")
					second := first
					if replay == "conflicting" {
						second.Approved = new(!approved)
						second.Reason = "later reason"
					}
					if replay == "another approval id" {
						second.ApprovalID = "another-approval"
					}
					msgs := repeatedApprovalMessages(first, second)
					if replay == "existing denial" {
						msgs[1].Content = append(msgs[1].Content, provider.ToolResultPart("call", "save", &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: new("first reason")}))
					}
					if replay == "converted UI" {
						var err error
						msgs, err = ConvertToModelMessages([]UIMessage{{ID: "assistant", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{
							ToolCallID: "call", ToolName: "save", Input: json.RawMessage(`{}`), State: ToolStateApprovalResponded,
							Approval: &ToolApproval{ID: "approval", Approved: new(approved), Reason: new("first reason")},
						}}}})
						require.NoError(t, err)
						msgs[len(msgs)-1].Content = append(msgs[len(msgs)-1].Content, second)
					}
					original, err := json.Marshal(msgs)
					require.NoError(t, err)
					var executed atomic.Int32
					var prompt []provider.Message
					model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
						prompt = opts.Prompt
						return &provider.StreamResult{Stream: textStreamParts("done")}, nil
					}}
					tools := ToolSet{"save": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
						executed.Add(1)
						return json.RawMessage(`"saved"`), nil
					}}}
					var responseMessages []provider.Message
					var parts []TextStreamPart
					switch api {
					case "stream":
						responseMessages, parts = collectStreamResult(t, StreamText(t.Context(), model, WithModelMessages(msgs...), WithTools(tools)))
					case "generate":
						result, err := GenerateText(t.Context(), model, WithModelMessages(msgs...), WithTools(tools))
						require.NoError(t, err)
						responseMessages = result.ResponseMessages
					case "agent stream":
						responseMessages, parts = collectStreamResult(t, NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(tools))).Stream(t.Context(), WithAgentModelMessages(msgs...)))
					case "agent generate":
						result, err := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(tools))).Generate(t.Context(), WithAgentModelMessages(msgs...))
						require.NoError(t, err)
						responseMessages = result.ResponseMessages
					}
					wantExecutions, wantDenials := int32(0), 1
					if approved {
						wantExecutions, wantDenials = 1, 0
					}
					assert.Equal(t, wantExecutions, executed.Load())
					assert.Equal(t, 1, model.callCount)
					if parts != nil {
						denials := 0
						for _, part := range parts {
							if _, ok := part.(StreamToolOutputDenied); ok {
								denials++
							}
						}
						assert.Equal(t, wantDenials, denials)
					}
					var results []provider.ContentPart
					for _, msg := range prompt {
						for _, part := range msg.Content {
							if part.Type == provider.ContentPartTypeToolResult {
								results = append(results, part)
							}
						}
					}
					require.Len(t, results, 1)
					assert.Equal(t, "call", results[0].ToolCallID)
					require.NotNil(t, results[0].Output)
					if approved {
						assert.NotEqual(t, provider.ToolOutputExecutionDenied, results[0].Output.Type)
					} else {
						assert.Equal(t, provider.ToolOutputExecutionDenied, results[0].Output.Type)
						assert.Equal(t, new("first reason"), results[0].Output.Reason)
					}
					if replay != "existing denial" && (replay != "converted UI" || approved) {
						require.NotEmpty(t, responseMessages)
						assert.Equal(t, []provider.ContentPart{results[0]}, responseMessages[0].Content)
					}
					after, err := json.Marshal(msgs)
					require.NoError(t, err)
					assert.JSONEq(t, string(original), string(after))
				})
			}
		}
	}
}
