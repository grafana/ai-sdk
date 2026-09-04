package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolExecutionFinishReason(t *testing.T) {
	tests := []struct {
		name       string
		reason     provider.UnifiedFinishReason
		shouldExec bool
	}{
		{name: "stop", reason: provider.FinishReasonStop, shouldExec: true},
		{name: "tool calls", reason: provider.FinishReasonToolCalls, shouldExec: true},
		{name: "length", reason: provider.FinishReasonLength},
		{name: "error", reason: provider.FinishReasonError},
		{name: "content filter", reason: provider.FinishReasonContentFilter},
		{name: "other", reason: provider.FinishReasonOther},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, generate := range []bool{false, true} {
				name := "stream"
				if generate {
					name = "generate"
				}
				t.Run(name, func(t *testing.T) {
					executions := 0
					model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
						stream := make(chan provider.StreamPart, 2)
						stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "write", Input: `{}`}
						stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: tc.reason}}
						close(stream)
						return &provider.StreamResult{Stream: stream}, nil
					}}
					tools := ToolSet{"write": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
						executions++
						return json.RawMessage(`{"ok":true}`), nil
					}}}
					if generate {
						_, err := GenerateText(context.Background(), model,
							WithModelMessages(provider.UserText("write")), WithTools(tools),
						)
						require.NoError(t, err)
					} else {
						result := StreamText(context.Background(), model,
							WithModelMessages(provider.UserText("write")), WithTools(tools),
						)
						for range result.FullStream() {
						}
					}
					if tc.shouldExec {
						assert.Equal(t, 1, executions)
					} else {
						assert.Zero(t, executions)
					}
				})
			}
		})
	}
}

func TestStreamText_UnsafeFinishDoesNotContinue(t *testing.T) {
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 2)
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "write", Input: `{}`}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonLength}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}
	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("write")),
		WithTools(ToolSet{"write": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		}}}),
		WithStopWhen(StepCountIs(3)),
	)
	for range result.FullStream() {
	}
	assert.NoError(t, result.Err())
	assert.Equal(t, 1, model.callCount)
}

func TestConvertToModelMessages_IgnoresPreliminaryOutput(t *testing.T) {
	messages := []UIMessage{
		{
			Role: RoleAssistant,
			Parts: []Part{
				StepStartPart{},
				ToolInvocationPart{
					ToolCallID:  "call-1",
					ToolName:    "lookup",
					State:       ToolStateOutputAvailable,
					Input:       json.RawMessage(`{"query":"test"}`),
					Output:      json.RawMessage(`{"result":"partial"}`),
					Preliminary: true,
				},
			},
		},
		{Role: RoleUser, Parts: []Part{TextPart{Text: "retry"}}},
	}

	got, err := ConvertToModelMessages(messages, WithIgnoreIncompleteToolCalls())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, provider.RoleUser, got[0].Role)
}

func TestAssembleUIMessage_ResetStep(t *testing.T) {
	msg, err := AssembleUIMessage(chunks(
		UIMessageChunk{Type: ChunkStartStep},
		TextStartChunk("completed"),
		TextDeltaChunk("completed", "completed"),
		TextEndChunk("completed"),
		UIMessageChunk{Type: ChunkFinishStep},
		UIMessageChunk{Type: ChunkStartStep},
		UIMessageChunk{Type: ChunkToolInputStart, ToolCallID: "stale", ToolName: "delete"},
		UIMessageChunk{Type: ChunkToolInputDelta, ToolCallID: "stale", InputTextDelta: `{"path":"partial`},
		UIMessageChunk{Type: ChunkResetStep},
		UIMessageChunk{Type: ChunkToolInputStart, ToolCallID: "retried", ToolName: "delete"},
		UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "retried", ToolName: "delete", Input: json.RawMessage(`{"path":"target"}`)},
	), WithUIMessageReaderGenerateID(func() string { return "message-1" }))
	require.NoError(t, err)
	require.Len(t, msg.Parts, 4)
	assert.IsType(t, StepStartPart{}, msg.Parts[0])
	assert.Equal(t, "completed", msg.Parts[1].(TextPart).Text)
	assert.IsType(t, StepStartPart{}, msg.Parts[2])
	assert.Equal(t, "retried", msg.Parts[3].(ToolInvocationPart).ToolCallID)
}

func TestAssembleUIMessage_PreservesPreliminaryOutput(t *testing.T) {
	msg, err := AssembleUIMessage(chunks(
		UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "call-1", ToolName: "lookup", Input: json.RawMessage(`{}`)},
		UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "call-1", Output: json.RawMessage(`{"partial":true}`), Preliminary: true},
	))
	require.NoError(t, err)
	require.Len(t, msg.Parts, 1)
	assert.True(t, msg.Parts[0].(ToolInvocationPart).Preliminary)
}

func TestStreamText_CallbackPanicsAreContained(t *testing.T) {
	t.Run("on chunk", func(t *testing.T) {
		model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("hello")}, nil
		}}
		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("hi")),
			OnChunk(func(OnChunkState) { panic("callback") }),
		)
		for range result.FullStream() {
		}
		assert.Equal(t, "hello", result.Text())
		assert.NoError(t, result.Err())
	})

	t.Run("on error", func(t *testing.T) {
		providerErr := errors.New("provider failed")
		model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart, 2)
			stream <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: providerErr.Error(), Cause: providerErr})}
			stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonError}}
			close(stream)
			return &provider.StreamResult{Stream: stream}, nil
		}}
		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("hi")),
			OnError(func(error) { panic("callback") }),
		)
		for range result.FullStream() {
		}
		assert.Error(t, result.Err())
	})
}
