package aisdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPruneMessages(t *testing.T) {
	data := provider.DataContent{Base64: "cG5n"}
	reasoning := provider.ReasoningPart("private summary")
	reasoningFile := provider.ReasoningFilePart("image/png", data)
	file := provider.FilePart("image/png", data)
	text := provider.TextPart("answer")
	custom := provider.CustomPart("example.item")
	user := provider.UserText("continue")

	t.Run("Defaults", func(t *testing.T) {
		history := []provider.Message{
			provider.NewSystemMessage(""),
			provider.NewAssistantMessage(reasoning, reasoningFile, pruneTestCall("c", "keep")),
			provider.NewToolMessage(pruneTestResult("c", "keep")),
			provider.NewUserMessage(),
		}
		assert.Equal(t, history[:3], PruneMessages(history))
		assert.Equal(t, history[:3], PruneMessages(history, nil, WithPruneReasoning(PruneReasoningNone)))
		assert.Nil(t, PruneMessages(nil))
	})

	t.Run("Reasoning", func(t *testing.T) {
		assistant := provider.NewAssistantMessage(reasoning, text, reasoningFile, file, custom)
		filtered := provider.NewAssistantMessage(text, file, custom)
		for _, tc := range []struct {
			name string
			mode PruneReasoningMode
			last provider.Message
			want provider.Message
		}{
			{"all", PruneReasoningAll, assistant, filtered},
			{"last assistant", PruneReasoningBeforeLastMessage, assistant, assistant},
			{"last user", PruneReasoningBeforeLastMessage, user, user},
		} {
			t.Run(tc.name, func(t *testing.T) {
				history := []provider.Message{assistant, tc.last}
				assert.Equal(t, []provider.Message{filtered, tc.want}, PruneMessages(history, WithPruneReasoning(tc.mode)))
			})
		}
		assert.Equal(t, []provider.Message{assistant}, PruneMessages([]provider.Message{assistant}, WithPruneReasoning(PruneReasoningAll), WithPruneReasoning(PruneReasoningNone)))
	})

	t.Run("ToolHistory", testPruneToolHistory)

	t.Run("ReasoningGroups", func(t *testing.T) {
		dropCall := pruneTestCall("d", "drop")
		keepCall := pruneTestCall("k", "keep")
		for _, tc := range []struct {
			name  string
			parts []provider.ContentPart
			want  []provider.ContentPart
		}{
			{"remove entire run", []provider.ContentPart{reasoning, reasoning, reasoningFile, dropCall}, nil},
			{"retained tool", []provider.ContentPart{reasoning, reasoningFile, dropCall, reasoningFile, reasoning, keepCall}, []provider.ContentPart{reasoningFile, reasoning, keepCall}},
			{"retained text", []provider.ContentPart{reasoning, reasoningFile, dropCall, text}, []provider.ContentPart{reasoning, reasoningFile, text}},
			{"trailing reasoning", []provider.ContentPart{dropCall, reasoning, reasoningFile}, []provider.ContentPart{reasoning, reasoningFile}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				history := []provider.Message{provider.NewAssistantMessage(tc.parts...), user}
				want := []provider.Message{user}
				if tc.want != nil {
					want = []provider.Message{provider.NewAssistantMessage(tc.want...), user}
				}
				assert.Equal(t, want, PruneMessages(history, WithPruneToolCalls(0, "drop")))
			})
		}
	})

	t.Run("EmptyMessages", func(t *testing.T) {
		preexisting := provider.NewToolMessage()
		blank := provider.NewAssistantMessage(provider.TextPart(""))
		history := []provider.Message{preexisting, provider.NewAssistantMessage(reasoning, reasoningFile), blank}
		prune := WithPruneReasoning(PruneReasoningAll)
		assert.Equal(t, []provider.Message{blank}, PruneMessages(history, prune))
		assert.Equal(t, []provider.Message{blank}, PruneMessages(history, prune, WithPruneEmptyMessages(false), WithPruneEmptyMessages(true)))
		assert.Equal(t, []provider.Message{preexisting, provider.NewAssistantMessage([]provider.ContentPart{}...), blank}, PruneMessages(history, prune, WithPruneEmptyMessages(false)))
		assert.Equal(t, history, PruneMessages(history, WithPruneEmptyMessages(false)))
	})

	t.Run("Ownership", func(t *testing.T) {
		opts := provider.ProviderOptions{"example": provider.RawProviderOption{Key: "example", Raw: json.RawMessage(`{"tag":"preserve"}`)}}
		file.ProviderOptions = opts
		dropCall := pruneTestCall("d", "drop")
		parts := []provider.ContentPart{reasoning, file, dropCall, custom}
		history := []provider.Message{{Role: provider.RoleAssistant, Content: parts[:3], ProviderOptions: opts}, user}
		got := PruneMessages(history, WithPruneReasoning(PruneReasoningAll), WithPruneToolCalls(0))
		assert.Equal(t, []provider.Message{{Role: provider.RoleAssistant, Content: []provider.ContentPart{file}, ProviderOptions: opts}, user}, got)
		require.Len(t, got, 2)
		require.Len(t, got[0].Content, 1)
		got[0].Content[0] = text
		got[1].Content[0] = text
		got[0] = user
		unchanged := PruneMessages(history)
		unchanged[0].Content[0] = text
		assert.Equal(t, provider.RoleAssistant, history[0].Role)
		assert.Equal(t, []provider.ContentPart{reasoning, file, dropCall, custom}, parts)
		assert.Equal(t, []provider.ContentPart{provider.TextPart("continue")}, history[1].Content)
	})

	t.Run("Continuation", testPruneContinuation)
}

func testPruneToolHistory(t *testing.T) {
	dropCall := pruneTestCall("d", "drop")
	dropResult := provider.ToolResultPart("d", "drop", &provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "denied"})
	dropReq := provider.ToolApprovalRequestPart("ad", "d", false)
	dropResp := provider.ToolApprovalResponsePart("ad", false, "no")
	keepCall := pruneTestCall("k", "keep")
	keepReq := provider.ToolApprovalRequestPart("ak", "k", true)
	keepResp := provider.ToolApprovalResponsePart("ak", true, "yes")
	keepResult := pruneTestResult("k", "keep")
	nativeCall := pruneTestCall("p", "drop")
	nativeCall.ProviderExecuted = true
	nativeResult := pruneTestResult("p", "drop")
	nativeResult.ProviderExecuted = true
	unresolved := provider.ToolApprovalResponsePart("unknown", true, "")
	text := provider.TextPart("working")
	user := provider.UserText("next")
	assistant := provider.NewAssistantMessage(text, dropCall, dropReq, keepCall, keepReq, nativeCall, nativeResult)
	tool := provider.NewToolMessage(keepResp, dropResp, unresolved, dropResult, keepResult)
	history := []provider.Message{assistant, tool, user}
	for _, tc := range []struct {
		name string
		opts []PruneOption
		want []provider.Message
	}{
		{"all", []PruneOption{WithPruneToolCalls(0)}, []provider.Message{provider.NewAssistantMessage(text), user}},
		{"before last user", []PruneOption{WithPruneToolCalls(1)}, []provider.Message{provider.NewAssistantMessage(text), user}},
		{"result and approval suffix", []PruneOption{WithPruneToolCalls(2)}, []provider.Message{provider.NewAssistantMessage(text, dropCall, dropReq, keepCall, keepReq), tool, user}},
		{"oversized suffix", []PruneOption{WithPruneToolCalls(10)}, history},
		{"negative count", []PruneOption{WithPruneToolCalls(-1)}, history},
		{"selected", []PruneOption{WithPruneToolCalls(0, "drop")}, []provider.Message{provider.NewAssistantMessage(text, keepCall, keepReq), provider.NewToolMessage(keepResp, keepResult), user}},
		{"explicit empty selector", []PruneOption{WithPruneToolCalls(0, []string{}...)}, []provider.Message{assistant, provider.NewToolMessage(keepResp, dropResp, dropResult, keepResult), user}},
		{"multiple rules", []PruneOption{WithPruneToolCalls(2, "drop"), WithPruneToolCalls(0, "keep")}, []provider.Message{provider.NewAssistantMessage(text, dropCall, dropReq), provider.NewToolMessage(dropResp, dropResult), user}},
	} {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, PruneMessages(history, tc.opts...)) })
	}

	t.Run("ApprovalProtectsEarlierResult", func(t *testing.T) {
		history := []provider.Message{
			provider.NewAssistantMessage(dropCall, dropReq, keepCall, keepReq),
			provider.NewToolMessage(dropResult, keepResult),
			provider.NewToolMessage(keepResp),
		}
		want := []provider.Message{
			provider.NewAssistantMessage(keepCall, keepReq),
			provider.NewToolMessage(keepResult),
			provider.NewToolMessage(keepResp),
		}
		assert.Equal(t, want, PruneMessages(history, WithPruneToolCalls(1)))
		assert.Equal(t, want, PruneMessages(history, WithPruneToolCalls(1, "drop", "keep")))
	})
	t.Run("ResultProtectsCallButNotEarlierApproval", func(t *testing.T) {
		history := []provider.Message{
			provider.NewAssistantMessage(keepCall, keepReq),
			provider.NewToolMessage(keepResp),
			provider.NewToolMessage(keepResult),
		}
		assert.Equal(t, []provider.Message{provider.NewAssistantMessage(keepCall), provider.NewToolMessage(keepResult)}, PruneMessages(history, WithPruneToolCalls(1)))
	})
	t.Run("CorrelatedDenialProtectsApproval", func(t *testing.T) {
		nativeCall := dropCall
		nativeCall.ProviderExecuted = true
		nativeDenial := provider.ProviderExecutedToolApprovalResponsePart("ad", false, "no")
		otherReq := provider.ToolApprovalRequestPart("other", "d", false)
		otherResp := provider.ProviderExecutedToolApprovalResponsePart("other", true, "yes")
		for _, tc := range []struct {
			name    string
			options provider.ProviderOptions
		}{
			{"matching approval", providerExecutedDenialOptions("ad")},
			{"unrelated approval", providerExecutedDenialOptions("ak")},
			{"unknown approval", providerExecutedDenialOptions("missing")},
			{"unmarked denial", nil},
		} {
			t.Run(tc.name, func(t *testing.T) {
				denial := provider.ToolResultPart("d", "drop", &provider.ToolResultOutput{
					Type: provider.ToolOutputExecutionDenied, ProviderOptions: tc.options,
				})
				results := provider.NewToolMessage(denial, keepResult)
				history := []provider.Message{
					provider.NewAssistantMessage(nativeCall, dropReq, otherReq, keepCall, keepReq),
					provider.NewToolMessage(nativeDenial, otherResp, keepResp),
					results,
				}
				want := []provider.Message{provider.NewAssistantMessage(nativeCall, keepCall), results}
				if tc.name == "matching approval" {
					want = []provider.Message{
						provider.NewAssistantMessage(nativeCall, dropReq, keepCall),
						provider.NewToolMessage(nativeDenial),
						results,
					}
				}
				assert.Equal(t, want, PruneMessages(history, WithPruneToolCalls(1)))
				assert.Empty(t, PruneMessages(history, WithPruneToolCalls(0)))
			})
		}
	})
	t.Run("CorrelatedDenialWithResultAlias", func(t *testing.T) {
		nativeCall := dropCall
		nativeCall.ProviderExecuted = true
		denial := provider.ToolResultPart("d", "alias", &provider.ToolResultOutput{
			Type: provider.ToolOutputExecutionDenied, ProviderOptions: providerExecutedDenialOptions("ad"),
		})
		response := provider.ProviderExecutedToolApprovalResponsePart("ad", false, "no")
		response.ToolCallID = nativeCall.ToolCallID
		history := []provider.Message{
			provider.NewAssistantMessage(nativeCall, dropReq),
			provider.NewToolMessage(response),
			provider.NewToolMessage(denial),
		}
		assert.Equal(t, history, PruneMessages(history, WithPruneToolCalls(0, "drop")))
	})
	t.Run("CorrelatedDenialDoesNotProtectConflictingDecision", func(t *testing.T) {
		nativeCall := dropCall
		nativeCall.ProviderExecuted = true
		denial := provider.ToolResultPart("d", "drop", &provider.ToolResultOutput{
			Type: provider.ToolOutputExecutionDenied, ProviderOptions: providerExecutedDenialOptions("ad"),
		})
		for _, tc := range []struct {
			name     string
			callID   string
			approved bool
		}{
			{name: "conflicting call", callID: "k"},
			{name: "approved decision", callID: "d", approved: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				decision := provider.ProviderExecutedToolApprovalResponsePart("ad", tc.approved, "")
				decision.ToolCallID = tc.callID
				result := provider.NewToolMessage(denial)
				history := []provider.Message{
					provider.NewAssistantMessage(nativeCall, dropReq),
					provider.NewToolMessage(decision),
					result,
				}
				assert.Equal(t, []provider.Message{provider.NewAssistantMessage(nativeCall), result}, PruneMessages(history, WithPruneToolCalls(1)))
			})
		}
	})
	t.Run("SuffixCountsBeforeRemovingEmpties", func(t *testing.T) {
		history := []provider.Message{provider.NewAssistantMessage(keepCall), provider.NewToolMessage(keepResult), provider.NewToolMessage(unresolved), user}
		assert.Equal(t, []provider.Message{user}, PruneMessages(history, WithPruneToolCalls(0, "drop"), WithPruneToolCalls(2, "keep")))
		assert.Equal(t, history[:3], PruneMessages(history[:3], WithPruneToolCalls(1, "drop")))
	})
}

func testPruneContinuation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pruned bool
		native bool
	}{
		{name: "unpruned local"},
		{name: "pruned local", pruned: true},
		{name: "pruned provider executed", pruned: true, native: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			history, err := ConvertToModelMessages([]UIMessage{
				{Role: RoleUser, Parts: []Part{TextPart{Text: "go"}}},
				{Role: RoleAssistant, Parts: []Part{
					ReasoningPart{Text: "old reasoning"},
					ReasoningFilePart{MediaType: "image/png", URL: "data:image/png;base64,cG5n"},
					TextPart{Text: "earlier answer"},
					ToolInvocationPart{ToolCallID: "old", ToolName: "old", State: ToolStateOutputAvailable, Input: json.RawMessage(`{}`), Output: json.RawMessage(`"old result"`)},
				}},
				{Role: RoleUser, Parts: []Part{TextPart{Text: "again"}}},
				{Role: RoleAssistant, Parts: []Part{ToolInvocationPart{
					ToolCallID: "current", ToolName: "current", State: ToolStateApprovalResponded, Input: json.RawMessage(`{}`),
					ProviderExecuted: tc.native, Approval: &ToolApproval{ID: "approve", Approved: new(true)},
				}}},
			})
			require.NoError(t, err)
			if tc.pruned {
				history = PruneMessages(history, WithPruneReasoning(PruneReasoningBeforeLastMessage), WithPruneToolCalls(1, "old", "current"))
			}
			var prompt []provider.Message
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				prompt = opts.Prompt
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			executed := 0
			result, err := GenerateText(t.Context(), model, WithModelMessages(history...), WithTools(ToolSet{
				"current": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					executed++
					return json.RawMessage(`"current result"`), nil
				}},
			}))
			require.NoError(t, err)
			assert.Equal(t, "done", result.Text)
			assert.Equal(t, 1, model.callCount)
			wantOld := []provider.ContentPartType{provider.ContentPartTypeReasoning, provider.ContentPartTypeReasoningFile, provider.ContentPartTypeText, provider.ContentPartTypeToolCall}
			wantMessages := 6
			if tc.pruned {
				wantOld = []provider.ContentPartType{provider.ContentPartTypeText}
				wantMessages--
			}
			require.Len(t, prompt, wantMessages)
			gotOld := make([]provider.ContentPartType, len(prompt[1].Content))
			for i, part := range prompt[1].Content {
				gotOld[i] = part.Type
			}
			assert.Equal(t, wantOld, gotOld)
			current := prompt[len(prompt)-2]
			require.Len(t, current.Content, 1)
			assert.Equal(t, "current", current.Content[0].ToolCallID)
			last := prompt[len(prompt)-1]
			require.Len(t, last.Content, 1)
			if tc.native {
				assert.Zero(t, executed)
				assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, last.Content[0].Type)
				assert.Equal(t, "approve", last.Content[0].ApprovalID)
				require.NotNil(t, last.Content[0].Approved)
				assert.True(t, *last.Content[0].Approved)
			} else {
				assert.Equal(t, 1, executed)
				assert.Equal(t, provider.ContentPartTypeToolResult, last.Content[0].Type)
				assert.Equal(t, "current", last.Content[0].ToolCallID)
			}
		})
	}
}

func pruneTestCall(id, name string) provider.ContentPart {
	return provider.ToolCallPart(id, name, json.RawMessage(`{}`))
}

func pruneTestResult(id, name string) provider.ContentPart {
	return provider.ToolResultPart(id, name, &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "result"})
}
