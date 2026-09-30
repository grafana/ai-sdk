package aisdk

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func effectiveToolChoiceParts(calls ...provider.StreamPart) []provider.StreamPart {
	parts := []provider.StreamPart{
		{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnOther, Message: "model warning"}}},
		{Type: provider.PartResponseMeta, ResponseID: "response-1", ModelID: "response-model", Timestamp: time.Unix(1, 0), ResponseHeaders: map[string]string{"request-id": "request-1"}},
		{Type: provider.PartReasoningStart, ID: "r1"},
		{Type: provider.PartReasoningDelta, ID: "r1", Delta: "thinking"},
		{Type: provider.PartReasoningEnd, ID: "r1"},
		{Type: provider.PartTextStart, ID: "t1"},
		{Type: provider.PartTextDelta, ID: "t1", Delta: "response"},
		{Type: provider.PartTextEnd, ID: "t1"},
	}
	parts = append(parts, calls...)
	return append(parts, provider.StreamPart{
		Type:             provider.PartFinish,
		FinishReason:     &provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: "tool_calls"},
		Usage:            &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)}},
		ProviderMetadata: provider.ProviderMetadata{"test": json.RawMessage(`{"requestId":"request-1"}`)},
	})
}

func effectiveToolChoiceModel(parts []provider.StreamPart) *mockModel {
	return &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			stream <- part
		}
		close(stream)
		return &provider.StreamResult{Stream: stream, Request: &provider.RequestMetadata{Body: json.RawMessage(`{"prompt":"test"}`)}}, nil
	}}
}

func TestStreamText_EffectiveToolChoiceViolation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		choice provider.ToolChoice
		calls  []provider.StreamPart
	}{
		{"required", provider.ToolChoice{Type: provider.ToolChoiceRequired}, nil},
		{"named", provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}, []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "other", Input: `{}`}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := effectiveToolChoiceModel(effectiveToolChoiceParts(tc.calls...))
			executions, errors, chunkErrors, stepFinishes, finishes := 0, 0, 0, 0, 0
			var errorCallbacks []string
			var callbackStep OnStepFinishState
			var callbackFinish OnFinishState
			result := StreamText(t.Context(), model,
				WithModelMessages(provider.UserText("test")), WithToolChoice(tc.choice), WithMaxRetries(2), WithStopWhen(StepCountIs(3)),
				WithTools(ToolSet{"lookup": {}, "other": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					executions++
					return json.RawMessage(`"unexpected"`), nil
				}}}),
				OnChunk(func(state OnChunkState) {
					if _, ok := state.Chunk.(StreamError); ok {
						chunkErrors++
						errorCallbacks = append(errorCallbacks, "chunk")
					}
				}),
				OnError(func(error) { errors++; errorCallbacks = append(errorCallbacks, "error") }),
				OnStepFinish(func(state OnStepFinishState) { stepFinishes++; callbackStep = state }),
				OnFinish(func(state OnFinishState) { finishes++; callbackFinish = state }),
			)
			var events []TextStreamPart
			for event := range result.FullStream() {
				events = append(events, event)
			}
			require.ErrorContains(t, result.Err(), "tool choice")
			assert.Zero(t, executions)
			assert.Equal(t, 1, model.callCount)
			assert.Equal(t, 1, errors)
			assert.Equal(t, 1, chunkErrors)
			assert.Equal(t, []string{"chunk", "error"}, errorCallbacks)
			assert.Equal(t, 1, stepFinishes)
			assert.Equal(t, 1, finishes)
			require.Len(t, result.Steps(), 1)
			step := result.Steps()[0]
			assert.Equal(t, provider.FinishReason{Unified: provider.FinishReasonError, Raw: "tool_calls"}, step.FinishReason)
			assert.Equal(t, step, callbackStep.StepResult)
			assert.Equal(t, step, callbackFinish.StepResult)
			assert.Equal(t, step.Usage, result.Usage())
			assert.Equal(t, step.Usage, result.TotalUsage())
			assert.Equal(t, step.Usage, callbackFinish.TotalUsage)
			require.NotNil(t, step.Usage.InputTokens.Total)
			assert.Equal(t, 10, *step.Usage.InputTokens.Total)
			require.NotNil(t, step.Usage.OutputTokens.Total)
			assert.Equal(t, 5, *step.Usage.OutputTokens.Total)
			assert.Equal(t, "response-1", result.Response().ID)
			assert.Equal(t, "response-model", result.Response().ModelID)
			assert.Equal(t, time.Unix(1, 0), result.Response().Timestamp)
			assert.Equal(t, map[string]string{"request-id": "request-1"}, result.Response().Headers)
			assert.Equal(t, provider.ProviderMetadata{"test": json.RawMessage(`{"requestId":"request-1"}`)}, result.ProviderMetadata())
			assert.Len(t, result.Warnings(), 1)
			assert.Equal(t, "response", step.Text)
			assert.Equal(t, "thinking", step.ReasoningText)
			assert.NotEmpty(t, step.Content)
			assert.NotEmpty(t, step.Response.Messages)
			assert.JSONEq(t, `{"prompt":"test"}`, string(step.Request.Body))
			require.GreaterOrEqual(t, len(events), 3)
			assert.IsType(t, StreamError{}, events[len(events)-3])
			assert.IsType(t, StreamFinishStep{}, events[len(events)-2])
			finish, ok := events[len(events)-1].(StreamFinish)
			require.True(t, ok)
			assert.Equal(t, provider.FinishReasonError, finish.FinishReason.Unified)
			assert.Equal(t, &step.Usage, finish.TotalUsage)
		})
	}
}
