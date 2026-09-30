package aisdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateText_EffectiveToolChoice(t *testing.T) {
	testEffectiveToolChoiceEntry(t, false, false)
}

func TestToolLoopAgent_EffectiveToolChoice(t *testing.T) {
	t.Run("stream", func(t *testing.T) { testEffectiveToolChoiceEntry(t, true, true) })
	t.Run("generate", func(t *testing.T) { testEffectiveToolChoiceEntry(t, true, false) })
}

func testEffectiveToolChoiceEntry(t *testing.T, useAgent, streaming bool) {
	t.Helper()
	required := provider.ToolChoice{Type: provider.ToolChoiceRequired}
	named := provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}
	for _, tc := range []struct {
		name      string
		choice    provider.ToolChoice
		override  *provider.ToolChoice
		callName  string
		wantError bool
	}{
		{"required miss", required, nil, "", true},
		{"named miss", named, nil, "other", true},
		{"required valid", required, nil, "lookup", false},
		{"named valid", named, nil, "lookup", false},
		{"step required miss", provider.ToolChoice{Type: provider.ToolChoiceAuto}, &required, "", true},
		{"step named miss", required, &named, "other", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []provider.StreamPart
			if tc.callName != "" {
				calls = append(calls, provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: tc.callName, Input: `{}`})
			}
			model := effectiveToolChoiceModel(effectiveToolChoiceParts(calls...))
			var finish OnFinishState
			var step OnStepFinishState
			stepFinishes, finishes, errors, executions := 0, 0, 0, 0
			opts := []Option{
				WithToolChoice(tc.choice), WithStopWhen(StepCountIs(1)),
				WithTools(ToolSet{"lookup": {}, "other": {NeedsApproval: ApprovalRequired(), Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					executions++
					return json.RawMessage(`"unexpected"`), nil
				}}}),
				OnError(func(error) { errors++ }),
				OnStepFinish(func(state OnStepFinishState) { stepFinishes++; step = state }),
				OnFinish(func(state OnFinishState) { finishes++; finish = state }),
			}
			if tc.override != nil {
				opts = append(opts, WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) {
					return &PrepareStepResult{ToolChoice: tc.override}, nil
				}))
			}
			var err error
			if useAgent {
				agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithToolChoice(provider.ToolChoice{Type: provider.ToolChoiceNone})))
				if streaming {
					result := agent.Stream(t.Context(), WithAgentPrompt("test"), WithAgentOptions(opts...))
					for range result.FullStream() {
					}
					err = result.Err()
					assert.Equal(t, finish.TotalUsage, result.TotalUsage())
					assert.Equal(t, step.ProviderMetadata, result.ProviderMetadata())
					assert.Equal(t, step.Response, result.Response())
				} else {
					var result *GenerateTextResult
					result, err = agent.Generate(t.Context(), WithAgentPrompt("test"), WithAgentOptions(opts...))
					if tc.wantError {
						assert.Nil(t, result)
					} else {
						require.NotNil(t, result)
					}
				}
			} else {
				generateOpts := []GenerateOption{WithModelMessages(provider.UserText("test"))}
				for _, opt := range opts {
					generateOpts = append(generateOpts, opt)
				}
				var result *GenerateTextResult
				result, err = GenerateText(t.Context(), model, generateOpts...)
				if tc.wantError {
					assert.Nil(t, result)
				} else {
					require.NotNil(t, result)
				}
			}
			if tc.wantError {
				require.ErrorContains(t, err, "tool choice")
				assert.Equal(t, provider.FinishReasonError, step.FinishReason.Unified)
				assert.Equal(t, 1, errors)
				assert.Empty(t, step.ToolApprovalRequests)
				assert.Empty(t, step.ToolResults)
			} else {
				require.NoError(t, err)
				assert.Zero(t, errors)
			}
			assert.Zero(t, executions)
			assert.Equal(t, 1, model.callCount)
			assert.Equal(t, 1, stepFinishes)
			assert.Equal(t, 1, finishes)
			require.Len(t, finish.Steps, 1)
			assert.Equal(t, step.StepResult, finish.StepResult)
			assert.Equal(t, "tool_calls", step.FinishReason.Raw)
			require.NotNil(t, step.Usage.InputTokens.Total)
			assert.Equal(t, 10, *step.Usage.InputTokens.Total)
			require.NotNil(t, step.Usage.OutputTokens.Total)
			assert.Equal(t, 5, *step.Usage.OutputTokens.Total)
			assert.Equal(t, step.Usage, finish.TotalUsage)
			assert.Equal(t, "response-1", step.Response.ID)
			assert.Equal(t, "response-model", step.Response.ModelID)
			assert.JSONEq(t, `{"requestId":"request-1"}`, string(step.ProviderMetadata["test"]))
			assert.Equal(t, "response", step.Text)
			assert.Equal(t, "thinking", step.ReasoningText)
		})
	}
}

func TestStreamText_EffectiveToolChoiceLifecycle(t *testing.T) {
	providerErr := provider.NewAPICallError(provider.APICallErrorOptions{Message: "provider failed"})
	for _, choice := range []provider.ToolChoice{{Type: provider.ToolChoiceRequired}, {Type: provider.ToolChoiceTool, ToolName: "lookup"}} {
		for _, tc := range []struct {
			name      string
			parts     []provider.StreamPart
			cancel    bool
			wantError error
			steps     int
		}{
			{name: "empty EOF", wantError: ErrNoOutputGenerated},
			{name: "partial EOF", parts: []provider.StreamPart{{Type: provider.PartTextStart, ID: "t1"}, {Type: provider.PartTextDelta, ID: "t1", Delta: "partial"}}, steps: 1},
			{name: "provider error", parts: []provider.StreamPart{{Type: provider.PartError, APICallError: providerErr}}, wantError: providerErr, steps: 1},
			{name: "provider error then finish", parts: []provider.StreamPart{{Type: provider.PartError, APICallError: providerErr}, {Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonError}}}, wantError: providerErr, steps: 1},
			{name: "cancel before finish", parts: []provider.StreamPart{{Type: provider.PartTextStart, ID: "t1"}, {Type: provider.PartTextDelta, ID: "t1", Delta: "partial"}}, cancel: true},
		} {
			t.Run(string(choice.Type)+"/"+tc.name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				model := effectiveToolChoiceModel(tc.parts)
				errors, aborts, streamErrors := 0, 0, 0
				result := StreamText(ctx, model, WithModelMessages(provider.UserText("test")), WithToolChoice(choice),
					OnError(func(error) { errors++ }), OnAbort(func(OnAbortState) { aborts++ }),
					OnChunk(func(state OnChunkState) {
						if _, ok := state.Chunk.(StreamTextDelta); ok && tc.cancel {
							cancel()
						}
					}))
				for event := range result.FullStream() {
					if _, ok := event.(StreamError); ok {
						streamErrors++
					}
				}
				if tc.wantError != nil {
					require.ErrorIs(t, result.Err(), tc.wantError)
					assert.Equal(t, 1, errors)
					assert.Equal(t, 1, streamErrors)
				} else {
					require.NoError(t, result.Err())
					assert.Zero(t, errors)
				}
				assert.Len(t, result.Steps(), tc.steps)
				assert.Equal(t, 1, model.callCount)
				if tc.cancel {
					assert.Equal(t, 1, aborts)
				} else {
					assert.Zero(t, aborts)
				}
			})
		}
	}
}
