package aisdk

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelStepNumbers_ApprovalResume(t *testing.T) {
	for _, entry := range []string{"StreamText", "GenerateText", "Agent.Stream", "Agent.Generate"} {
		t.Run(entry, func(t *testing.T) {
			var trace []string
			model := &mockModel{}
			model.streamFunc = func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				trace = append(trace, "model")
				if model.callCount == 1 && assert.NotEmpty(t, opts.Prompt) {
					last := opts.Prompt[len(opts.Prompt)-1]
					assert.Equal(t, provider.RoleTool, last.Role)
					if assert.Len(t, last.Content, 1) {
						assert.Equal(t, provider.ContentPartTypeToolResult, last.Content[0].Type)
						assert.Equal(t, "from-history", last.Content[0].ToolCallID)
					}
				}
				if model.callCount <= 2 {
					id := fmt.Sprintf("c%d", model.callCount)
					return &provider.StreamResult{Stream: discoveryParts(
						discoveryCall(id, "lookup", `{}`),
						provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}},
					)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}

			messages := []provider.Message{
				provider.UserText("look up"),
				provider.NewAssistantMessage(
					provider.ToolCallPart("from-history", "lookup", json.RawMessage(`{}`)),
					provider.ToolApprovalRequestPart("approval", "from-history", false),
				),
				provider.NewToolMessage(provider.ToolApprovalResponsePart("approval", true, "")),
			}
			tools := ToolSet{"lookup": {Execute: func(_ context.Context, _ json.RawMessage, opts ToolExecutionOptions) (json.RawMessage, error) {
				trace = append(trace, "execute "+opts.ToolCallID)
				return json.RawMessage(`"found"`), nil
			}}}

			noteTool := func(event string, n int, stepModel StepModel, call ToolCall) {
				wantModel := StepModel{Provider: model.Provider(), ModelID: model.ModelID()}
				if call.ToolCallID == "from-history" {
					wantModel = StepModel{}
				}
				assert.Equal(t, wantModel, stepModel)
				trace = append(trace, fmt.Sprintf("%s %s %d", event, call.ToolCallID, n))
			}
			var finish OnFinishState
			opts := []Option{
				WithTools(tools),
				WithPrepareStep(func(s PrepareStepState) (*PrepareStepResult, error) {
					trace = append(trace, fmt.Sprintf("prepare %d", s.StepNumber))
					assert.Equal(t, len(s.Steps), s.StepNumber)
					return nil, nil
				}),
				OnStepStart(func(s OnStepStartState) { trace = append(trace, fmt.Sprintf("start %d", s.StepNumber)) }),
				OnToolCallStart(func(s OnToolCallStartState) { noteTool("tool start", s.StepNumber, s.Model, s.ToolCall) }),
				OnToolCallFinish(func(s OnToolCallFinishState) { noteTool("tool finish", s.StepNumber, s.Model, s.ToolCall) }),
				OnStepFinish(func(s OnStepFinishState) { trace = append(trace, fmt.Sprintf("step finish %d", s.StepNumber)) }),
				WithStopWhen(func(s StopConditionState) bool {
					trace = append(trace, fmt.Sprintf("stop check %d", s.Steps[len(s.Steps)-1].StepNumber))
					return StepCountIs(3)(s)
				}),
				OnFinish(func(s OnFinishState) { finish = s }),
			}

			streamOpts := make([]StreamOption, len(opts))
			generateOpts := make([]GenerateOption, len(opts))
			for i, opt := range opts {
				streamOpts[i] = opt
				generateOpts[i] = opt
			}
			var steps []StepResult
			var streamed *StreamTextResult
			switch entry {
			case "StreamText":
				streamed = StreamText(t.Context(), model, append(streamOpts, WithModelMessages(messages...))...)
			case "GenerateText":
				result, err := GenerateText(t.Context(), model, append(generateOpts, WithModelMessages(messages...))...)
				require.NoError(t, err)
				steps = result.Steps
			case "Agent.Stream":
				agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(streamOpts...))
				streamed = agent.Stream(t.Context(), WithAgentModelMessages(messages...))
			case "Agent.Generate":
				agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(streamOpts...))
				result, err := agent.Generate(t.Context(), WithAgentModelMessages(messages...))
				require.NoError(t, err)
				steps = result.Steps
			}
			if streamed != nil {
				for range streamed.FullStream() {
				}
				require.NoError(t, streamed.Err())
				steps = streamed.Steps()
			}

			assert.Equal(t, []string{
				"tool start from-history -1", "execute from-history", "tool finish from-history -1",
				"prepare 0", "start 0", "model", "tool start c1 0", "execute c1", "tool finish c1 0", "step finish 0", "stop check 0",
				"prepare 1", "start 1", "model", "tool start c2 1", "execute c2", "tool finish c2 1", "step finish 1", "stop check 1",
				"prepare 2", "start 2", "model", "step finish 2", "stop check 2",
			}, trace)
			require.Len(t, steps, 3)
			assert.Equal(t, []int{0, 1, 2}, []int{steps[0].StepNumber, steps[1].StepNumber, steps[2].StepNumber})
			assert.Equal(t, []StepType{StepTypeInitial, StepTypeToolResult, StepTypeToolResult}, []StepType{steps[0].StepType, steps[1].StepType, steps[2].StepType})
			assert.Equal(t, steps, finish.Steps)
			assert.Equal(t, 2, finish.StepNumber)
		})
	}
}
