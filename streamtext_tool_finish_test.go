package aisdk

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamText_ToolExecutionFinishReason(t *testing.T) {
	for _, reason := range []provider.UnifiedFinishReason{provider.FinishReasonStop, provider.FinishReasonToolCalls, provider.FinishReasonLength, provider.FinishReasonContentFilter, provider.FinishReasonError, provider.FinishReasonInterrupted, provider.FinishReasonOther, ""} {
		for _, approval := range []ToolApprovalStatus{ToolApprovalNotApplicable, ToolApprovalApproved, ToolApprovalUserApproval, ToolApprovalDenied} {
			t.Run(string(reason)+"/"+string(approval), func(t *testing.T) {
				var requests, executions, decisions atomic.Int32
				model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					parts := make(chan provider.StreamPart, 2)
					if requests.Add(1) == 1 {
						parts <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call_1", ToolName: "action", Input: `{}`}
						if reason != "" {
							parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: reason}}
						}
					} else {
						parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
					}
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				}}
				result := StreamText(t.Context(), model, WithStopWhen(StepCountIs(2)), WithTools(ToolSet{"action": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					executions.Add(1)
					return json.RawMessage(`"done"`), nil
				}}}), WithToolApproval(ToolApprovalFunc(func(ToolApprovalOptions) (ToolApprovalDecision, error) {
					decisions.Add(1)
					return ToolApprovalDecision{Status: approval}, nil
				})))
				var calls, approvalRequests, approvalResponses int
				for part := range result.FullStream() {
					switch part.(type) {
					case StreamToolCall:
						calls++
					case StreamToolApprovalRequest:
						approvalRequests++
					case StreamToolApprovalResponse:
						approvalResponses++
					}
				}
				require.NoError(t, result.Err())
				allowed := reason == provider.FinishReasonStop || reason == provider.FinishReasonToolCalls
				wantExecutions := int32(0)
				if allowed && (approval == ToolApprovalNotApplicable || approval == ToolApprovalApproved) {
					wantExecutions = 1
				}
				wantRequests := int32(1)
				if reason != provider.FinishReasonInterrupted && (wantExecutions == 1 || approval == ToolApprovalDenied) {
					wantRequests = 2
				}
				assert.Equal(t, wantExecutions, executions.Load())
				assert.Equal(t, wantRequests, requests.Load())
				assert.Equal(t, int32(1), decisions.Load())
				assert.Equal(t, 1, calls)
				if approval == ToolApprovalNotApplicable {
					assert.Zero(t, approvalRequests)
				} else {
					assert.Equal(t, 1, approvalRequests)
				}
				if approval == ToolApprovalApproved || approval == ToolApprovalDenied {
					assert.Equal(t, 1, approvalResponses)
				} else {
					assert.Zero(t, approvalResponses)
				}
			})
		}
	}
}

func TestStreamText_InterruptedPreservesOutput(t *testing.T) {
	for _, choice := range []provider.ToolChoice{{Type: provider.ToolChoiceRequired}, {Type: provider.ToolChoiceTool, ToolName: "lookup"}} {
		t.Run(string(choice.Type), func(t *testing.T) {
			usage := &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(3)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(2)}}
			metadata := provider.ProviderMetadata{"test": json.RawMessage(`{"partial":true}`)}
			model := effectiveToolChoiceModel([]provider.StreamPart{
				{Type: provider.PartTextStart, ID: "1"},
				{Type: provider.PartTextDelta, ID: "1", Delta: "Let me check"},
				{Type: provider.PartTextEnd, ID: "1"},
				{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonInterrupted}, Usage: usage, ProviderMetadata: metadata},
			})
			var errs []error
			var stepFinishes, finishes []provider.UnifiedFinishReason
			result := StreamText(t.Context(), model,
				WithModelMessages(provider.UserText("test")),
				WithToolChoice(choice),
				WithTools(ToolSet{"lookup": {}}),
				OnError(func(err error) { errs = append(errs, err) }),
				OnStepFinish(func(state OnStepFinishState) { stepFinishes = append(stepFinishes, state.FinishReason.Unified) }),
				OnFinish(func(state OnFinishState) { finishes = append(finishes, state.FinishReason.Unified) }),
			)
			var finishChunk *UIMessageChunk
			for chunk := range result.ToUIMessageStream() {
				switch chunk.Type {
				case ChunkError:
					t.Fatalf("unexpected error chunk: %s", chunk.ErrorText)
				case ChunkFinish:
					finishChunk = &chunk
				}
			}

			require.NoError(t, result.Err())
			assert.Empty(t, errs)
			assert.Equal(t, "Let me check", result.Text())
			assert.Equal(t, provider.FinishReason{Unified: provider.FinishReasonInterrupted}, result.FinishReason())
			assert.Equal(t, *usage, result.Usage())
			assert.Equal(t, metadata, result.ProviderMetadata())
			assert.Equal(t, []provider.UnifiedFinishReason{provider.FinishReasonInterrupted}, stepFinishes)
			assert.Equal(t, []provider.UnifiedFinishReason{provider.FinishReasonInterrupted}, finishes)
			require.NotNil(t, finishChunk)
			assert.Equal(t, string(provider.FinishReasonInterrupted), finishChunk.FinishReason)
			assert.Equal(t, 1, model.callCount)
		})
	}
}
