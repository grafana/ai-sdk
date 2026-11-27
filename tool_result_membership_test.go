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

func toolResultIDs(results []ToolResult) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.ToolCallID)
	}
	return ids
}

func TestToolResults_FailedExecutionProjection(t *testing.T) {
	for _, name := range []string{"stream", "generate", "agent stream", "agent generate"} {
		t.Run(name, func(t *testing.T) {
			failure := errors.New("inventory unavailable")
			var nextPrompt []provider.Message
			model := &mockModel{}
			model.streamFunc = func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				if model.callCount == 1 {
					return &provider.StreamResult{Stream: toolCallStreamParts("inventory", `{}`)}, nil
				}
				nextPrompt = opts.Prompt
				return &provider.StreamResult{Stream: textStreamParts("Recovered")}, nil
			}
			var finishedSteps []OnStepFinishState
			var stopResults [][][]string
			var finish OnFinishState
			var toolFinish OnToolCallFinishState
			opts := []Option{
				WithTools(ToolSet{"inventory": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					return nil, failure
				}}}),
				WithStopWhen(func(state StopConditionState) bool {
					var ids [][]string
					for _, step := range state.Steps {
						ids = append(ids, toolResultIDs(step.ToolResults))
					}
					stopResults = append(stopResults, ids)
					return len(state.Steps) == 2
				}),
				OnStepFinish(func(state OnStepFinishState) { finishedSteps = append(finishedSteps, state) }),
				OnFinish(func(state OnFinishState) { finish = state }),
				OnToolCallFinish(func(state OnToolCallFinishState) { toolFinish = state }),
			}
			consume := func(result *StreamTextResult) projectedResult {
				t.Helper()
				var errorIDs []string
				if name == "agent stream" {
					for chunk := range result.ToUIMessageStream(WithUIMessageStreamRedactToolErrors(false)) {
						if chunk.Type == ChunkToolOutputError {
							assert.Equal(t, failure.Error(), chunk.ErrorText)
							errorIDs = append(errorIDs, chunk.ToolCallID)
						}
					}
				} else {
					for part := range result.FullStream() {
						if toolErr, ok := part.(StreamToolError); ok {
							assert.ErrorIs(t, toolErr.Error, failure)
							errorIDs = append(errorIDs, toolErr.ToolCallID)
						}
					}
				}
				assert.Equal(t, []string{"c1"}, errorIDs)
				require.NoError(t, result.Err())
				return projectStream(result)
			}
			streamOpts := make([]StreamOption, 0, len(opts)+1)
			generateOpts := make([]GenerateOption, 0, len(opts)+1)
			for _, opt := range opts {
				streamOpts = append(streamOpts, opt)
				generateOpts = append(generateOpts, opt)
			}
			var result projectedResult
			switch name {
			case "stream":
				result = consume(StreamText(t.Context(), model, append(streamOpts, WithModelMessages(provider.UserText("inventory?")))...))
			case "generate":
				generated, err := GenerateText(t.Context(), model, append(generateOpts, WithModelMessages(provider.UserText("inventory?")))...)
				require.NoError(t, err)
				result = projectGenerate(generated)
			case "agent stream":
				agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(streamOpts...))
				result = consume(agent.Stream(t.Context(), WithAgentPrompt("inventory?")))
			case "agent generate":
				agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(streamOpts...))
				generated, err := agent.Generate(t.Context(), WithAgentPrompt("inventory?"))
				require.NoError(t, err)
				result = projectGenerate(generated)
			}

			assert.Equal(t, "Recovered", result.text)
			assert.Equal(t, 2, model.callCount)
			require.Len(t, result.steps, 2)
			require.Len(t, finishedSteps, 2)
			require.Len(t, stopResults, 2)
			require.Len(t, finish.Steps, 2)
			assert.Equal(t, 0, result.steps[0].StepNumber)
			assert.Equal(t, 1, result.steps[1].StepNumber)
			assert.Equal(t, "c1", toolFinish.ToolCall.ToolCallID)
			assert.Equal(t, 0, toolFinish.StepNumber)
			assert.False(t, toolFinish.Success)
			assert.ErrorIs(t, toolFinish.Error, failure)

			for _, content := range [][]ContentPart{result.content, result.steps[0].Content, finishedSteps[0].Content, finish.Content} {
				var found bool
				for _, part := range content {
					if toolErr, ok := part.(ToolErrorContent); ok && toolErr.ToolCallID == "c1" {
						assert.Equal(t, failure, toolErr.Error)
						found = true
					}
				}
				assert.True(t, found)
			}
			for _, messages := range [][]provider.Message{nextPrompt, result.steps[0].Response.Messages, result.responseMessages, finish.ResponseMessages} {
				outcomes := promptToolOutputs(messages)
				require.Contains(t, outcomes, "c1")
				assert.Equal(t, &provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: failure.Error()}, outcomes["c1"])
			}

			assert.Empty(t, result.steps[0].ToolResults, "recorded step")
			assert.Empty(t, result.toolResults, "collected result")
			assert.Empty(t, finishedSteps[0].ToolResults, "OnStepFinish")
			assert.Empty(t, finish.Steps[0].ToolResults, "OnFinish Steps")
			assert.Empty(t, finish.ToolResults, "OnFinish aggregate")
			assert.Empty(t, stopResults[0][0], "StopWhen")
			assert.Empty(t, stopResults[1][0], "StopWhen historical step")
		})
	}
}

func promptToolOutputs(messages []provider.Message) map[string]*provider.ToolResultOutput {
	outputs := map[string]*provider.ToolResultOutput{}
	for _, msg := range messages {
		for _, part := range msg.Content {
			if part.Type == provider.ContentPartTypeToolResult {
				outputs[part.ToolCallID] = part.Output
			}
		}
	}
	return outputs
}

func TestToolResults_MixedContentKinds(t *testing.T) {
	call := func(id, name string, hosted bool) provider.StreamPart {
		return provider.StreamPart{Type: provider.PartToolCall, ToolCallID: id, ToolName: name, Input: `{}`, ProviderExecuted: hosted}
	}
	providerResult := func(id string, failed, preliminary bool) provider.StreamPart {
		return provider.StreamPart{Type: provider.PartToolResult, ToolCallID: id, ToolName: "hosted", ProviderExecuted: true,
			Result: json.RawMessage(`{"message":"error-looking data"}`), IsError: failed, Preliminary: &preliminary}
	}
	ordinary := json.RawMessage(`{"error":"ordinary payload"}`)
	model := &mockModel{}
	var nextPrompt []provider.Message
	model.streamFunc = func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		if model.callCount == 1 {
			return &provider.StreamResult{Stream: projectionParts(
				call("ordinary", "ordinary", false),
				call("denied", "denied", false),
				call("invalid", "missing", false),
				call("hosted-ok", "hosted", true),
				call("hosted-error", "hosted", true),
				call("hosted-preview", "hosted", true),
				providerResult("hosted-preview", false, true),
				providerResult("hosted-error", true, false),
				providerResult("hosted-ok", false, false),
				provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}},
			)}, nil
		}
		nextPrompt = opts.Prompt
		return &provider.StreamResult{Stream: textStreamParts("Recovered")}, nil
	}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("lookup")), WithStopWhen(StepCountIs(2)),
		WithTools(ToolSet{
			"ordinary": {
				Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					return ordinary, nil
				},
				ToModelOutput: func(ToolOutputContext) (*provider.ToolResultOutput, error) {
					return &provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "reported by application"}, nil
				},
			},
			"denied": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				t.Error("denied tool executed")
				return nil, nil
			}},
		}), WithToolApproval(ToolApprovalMap{"denied": ApprovalPolicy(ToolApprovalDenied, "not authorized")}))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	assert.Equal(t, "Recovered", result.Text())
	steps := result.Steps()
	require.Len(t, steps, 2)
	assert.Equal(t, []string{"ordinary", "hosted-ok"}, toolResultIDs(steps[0].ToolResults))
	assert.Equal(t, []string{"ordinary", "hosted-ok"}, toolResultIDs(result.ToolResults()))
	for _, result := range steps[0].ToolResults {
		if result.ToolCallID == "ordinary" {
			assert.Equal(t, ordinary, result.Output)
			assert.False(t, result.IsError)
		}
	}
	outputs := promptToolOutputs(nextPrompt)
	require.Contains(t, outputs, "invalid")
	assert.Equal(t, provider.ToolOutputErrorText, outputs["invalid"].Type)
	require.Contains(t, outputs, "denied")
	assert.Equal(t, provider.ToolOutputExecutionDenied, outputs["denied"].Type)
	require.Contains(t, outputs, "hosted-error")
	assert.Equal(t, provider.ToolOutputErrorJSON, outputs["hosted-error"].Type)
	assert.NotContains(t, outputs, "hosted-preview")
	assert.Contains(t, outputs, "hosted-ok")
	require.Contains(t, outputs, "ordinary")
	assert.Equal(t, "reported by application", outputs["ordinary"].Text)
}
