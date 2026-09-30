package aisdk

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIMessageReader_ProviderExecutionPresence(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		for _, kind := range []ChunkType{ChunkToolApprovalResponse, ChunkToolOutputAvailable, ChunkToolOutputError} {
			for _, supplied := range []bool{false, true} {
				t.Run(fmt.Sprintf("%t/%s/false-supplied-%t", dynamic, kind, supplied), func(t *testing.T) {
					initial := toolPartFields{ToolCallID: "c", ToolName: "lookup", State: ToolStateApprovalRequested, Input: json.RawMessage(`{}`), ProviderExecuted: true, Approval: &ToolApproval{ID: "a", Descriptor: json.RawMessage(`{"scope":"all"}`), RequestReason: new("request"), Signature: "sig", IsAutomatic: true}}
					var part Part = ToolInvocationPart(initial)
					if dynamic {
						part = DynamicToolUIPart(initial)
					}
					fields := `"toolCallId":"c","approvalId":"a","approved":true,"output":"ok","errorText":""`
					if supplied {
						fields += `,"providerExecuted":false`
					}
					var chunk UIMessageChunk
					require.NoError(t, json.Unmarshal([]byte(`{"type":"`+string(kind)+`",`+fields+`}`), &chunk))
					input := []UIMessageChunk{chunk}
					if kind == ChunkToolApprovalResponse {
						input = append(input, UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "c", Output: json.RawMessage(`"ok"`)})
					}
					message, err := AssembleUIMessage(chunks(input...), WithUIMessageReaderInitialMessage(UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{part}}))
					require.NoError(t, err)
					var got toolPartFields
					switch p := message.Parts[0].(type) {
					case ToolInvocationPart:
						got = toolPartFields(p)
					case DynamicToolUIPart:
						got = toolPartFields(p)
					}
					assert.Equal(t, !supplied, got.ProviderExecuted)
					assert.Equal(t, "sig", got.Approval.Signature)
					models, err := ConvertToModelMessages([]UIMessage{message})
					require.NoError(t, err)
					if supplied {
						require.Len(t, models, 2)
						assert.Equal(t, provider.RoleTool, models[1].Role)
					} else {
						assert.Equal(t, provider.ContentPartTypeToolResult, models[0].Content[len(models[0].Content)-1].Type)
						if kind == ChunkToolApprovalResponse {
							require.Len(t, models, 2)
							assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, models[1].Content[0].Type)
						} else {
							require.Len(t, models, 1)
						}
					}
				})
			}
		}
	}
}

func TestUIMessageReader_StaticErrorContinuation(t *testing.T) {
	for _, seeded := range []bool{false, true} {
		for _, raw := range []json.RawMessage{json.RawMessage(`"legacy"`), json.RawMessage(`{"q":"legacy"}`), json.RawMessage(`null`)} {
			t.Run(fmt.Sprintf("seeded-%t/%s", seeded, raw), func(t *testing.T) {
				var options []UIMessageReaderOption
				var updates []UIMessageChunk
				if seeded {
					options = append(options, WithUIMessageReaderInitialMessage(UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{ToolCallID: "c", ToolName: "lookup", State: ToolStateOutputError, RawInput: raw, ErrorText: new("old")}}}))
				} else {
					updates = append(updates, UIMessageChunk{Type: ChunkToolInputError, ToolCallID: "c", ToolName: "lookup", Input: raw, ErrorText: "old"})
				}
				updates = append(updates, UIMessageChunk{Type: ChunkToolOutputError, ToolCallID: "c"})
				snapshots := collectMessages(StreamUIMessage(chunks(updates...), options...))
				require.NotEmpty(t, snapshots)
				for _, snapshot := range snapshots {
					part := snapshot.Parts[0].(ToolInvocationPart)
					assert.JSONEq(t, string(raw), string(part.RawInput))
				}
				message, err := AssembleUIMessage(chunks(updates...), options...)
				require.NoError(t, err)
				part := message.Parts[0].(ToolInvocationPart)
				assert.JSONEq(t, string(raw), string(part.RawInput))
				assert.Nil(t, part.Input)
				models, err := ConvertToModelMessages([]UIMessage{message})
				require.NoError(t, err)
				assert.JSONEq(t, string(raw), string(models[0].Content[0].Input))
			})
		}
	}
}

func TestUIMessageReader_InitialMessageIsolation(t *testing.T) {
	initial := UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{ToolCallID: "c", ToolName: "lookup", State: ToolStateOutputError, Title: new("title"), Input: json.RawMessage(`{}`), RawInput: json.RawMessage(`"raw"`), ErrorText: new("error"), ToolMetadata: map[string]json.RawMessage{"scope": json.RawMessage(`{"name":"all"}`)}, Approval: &ToolApproval{ID: "a", Approved: new(true), Reason: new("reason"), Descriptor: json.RawMessage(`{"scope":"all"}`)}}}}
	before, err := json.Marshal(initial)
	require.NoError(t, err)
	option := WithUIMessageReaderInitialMessage(initial)
	part := initial.Parts[0].(ToolInvocationPart)
	*part.Title = "mutated"
	part.ToolMetadata["scope"][0] = 'x'
	part.Approval.Descriptor[0] = 'x'
	message, err := AssembleUIMessage(chunks(), option)
	require.NoError(t, err)
	encoded, err := json.Marshal(message)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(encoded))
	part = message.Parts[0].(ToolInvocationPart)
	*part.ErrorText = "changed"
	*part.Approval.Reason = "changed"
	part.RawInput[0] = 'x'
	again, err := AssembleUIMessage(chunks(), option)
	require.NoError(t, err)
	encoded, err = json.Marshal(again)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(encoded))
	assert.Empty(t, collectMessages(StreamUIMessage(chunks(), option)))
	_, err = AssembleUIMessage(chunks(TextDeltaChunk("t", "invalid")), option)
	require.Error(t, err)
	for _, id := range []string{"user-id", ""} {
		seed := WithUIMessageReaderInitialMessage(UIMessage{ID: id, Role: RoleUser, Metadata: json.RawMessage(`{"ignored":true}`), Parts: []Part{TextPart{Text: "ignored"}}})
		got, err := AssembleUIMessage(chunks(), seed, WithUIMessageReaderGenerateID(func() string { return "generated" }))
		require.NoError(t, err)
		want := id
		if want == "" {
			want = "generated"
		}
		assert.Equal(t, want, got.ID)
		assert.Empty(t, got.Parts)
		assert.Empty(t, got.Metadata)
		got, err = AssembleUIMessage(chunks(UIMessageChunk{Type: ChunkStart, MessageID: "replacement"}), seed)
		require.NoError(t, err)
		assert.Equal(t, "replacement", got.ID)
	}
	t.Run("data replacement", func(t *testing.T) {
		seed := WithUIMessageReaderInitialMessage(UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{DataPart{DataName: "status", ID: "d", Data: json.RawMessage(`1`)}}})
		got, err := AssembleUIMessage(chunks(UIMessageChunk{Type: ChunkData, DataName: "status", ID: "d", Data: json.RawMessage(`2`)}), seed)
		require.NoError(t, err)
		require.Len(t, got.Parts, 1)
		assert.JSONEq(t, `2`, string(got.Parts[0].(DataPart).Data))
	})
}
