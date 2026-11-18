package aisdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActiveToolsRejectInactiveCall(t *testing.T) {
	for _, tc := range []struct {
		name      string
		selection Option
		input     string
		wantFirst []string
		wantNext  []string
		available string
	}{
		{"global", WithActiveTools("a"), `{}`, []string{"a"}, []string{"a"}, "a"},
		{"malformed input", WithActiveTools("a"), `{`, []string{"a"}, []string{"a"}, "a"},
		{"step override", WithPrepareStep(func(p PrepareStepState) (*PrepareStepResult, error) {
			if p.StepNumber == 0 {
				return &PrepareStepResult{ActiveTools: []string{"a"}}, nil
			}
			return nil, nil
		}), `{}`, []string{"a"}, []string{"a", "b"}, "a"},
		{"step empty", WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) {
			return &PrepareStepResult{ActiveTools: []string{}}, nil
		}), `{}`, nil, nil, ""},
	} {
		for _, setup := range []struct {
			name   string
			search bool
			routes ToolRoutes
		}{
			{name: "plain"},
			{name: "search", search: true},
			{name: "routes", routes: ToolRoutes{"a": {Direct: true}}},
			{name: "search and routes", search: true, routes: ToolRoutes{"a": {Direct: true}}},
		} {
			for _, entry := range []string{"stream", "generate", "agent"} {
				t.Run(tc.name+"/"+setup.name+"/"+entry, func(t *testing.T) {
					const callID = "c1"
					wantError := "AI_NoSuchToolError: Model tried to call unavailable tool 'b'. Available tools: " + tc.available + "."
					var calls, inputCallbacks, approvalChecks, executions int
					var nextPrompt []provider.Message
					tools := ToolSet{"a": {}, "b": {
						OnInputAvailable: func(json.RawMessage, ToolExecutionOptions) { inputCallbacks++ },
						NeedsApproval: ApprovalIf(func(json.RawMessage, ToolExecutionOptions) (bool, error) {
							approvalChecks++
							return false, nil
						}),
						Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
							executions++
							return json.RawMessage(`"ran"`), nil
						},
					}}
					if setup.search {
						tools["deferred"] = Tool{DeferLoading: true}
					}
					model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
						calls++
						if calls == 1 {
							assert.Equal(t, tc.wantFirst, toolNames(opts.Tools))
							return &provider.StreamResult{Stream: toolCallStreamParts("b", tc.input)}, nil
						}
						assert.Equal(t, tc.wantNext, toolNames(opts.Tools))
						nextPrompt = opts.Prompt
						return &provider.StreamResult{Stream: textStreamParts("recovered")}, nil
					}}

					ctx := t.Context()
					messages := WithModelMessages(provider.UserText("go"))
					registry, routes, stop := WithTools(tools), WithToolRoutes(setup.routes), WithStopWhen(StepCountIs(2))
					var steps []StepResult
					var chunks []UIMessageChunk
					switch entry {
					case "stream":
						result := StreamText(ctx, model, messages, registry, routes, stop, tc.selection)
						chunks = collectUIChunks(result.ToUIMessageStream())
						require.NoError(t, result.Err())
						steps = result.Steps()
					case "generate":
						result, err := GenerateText(ctx, model, messages, registry, routes, stop, tc.selection)
						require.NoError(t, err)
						steps = result.Steps
					case "agent":
						agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(registry, routes, stop, tc.selection))
						result, err := agent.Generate(ctx, WithAgentPrompt("go"))
						require.NoError(t, err)
						steps = result.Steps
					}

					assert.Zero(t, inputCallbacks)
					assert.Zero(t, approvalChecks)
					assert.Zero(t, executions)
					require.Len(t, steps, 2)
					require.Len(t, steps[0].ToolCalls, 1)
					assert.True(t, steps[0].ToolCalls[0].Invalid)
					assert.EqualError(t, steps[0].ToolCalls[0].Error, wantError)
					var outputs []provider.ToolResultOutput
					for _, message := range nextPrompt {
						for _, part := range message.Content {
							if part.Type == provider.ContentPartTypeToolResult && part.ToolCallID == callID && part.Output != nil {
								outputs = append(outputs, *part.Output)
							}
						}
					}
					assert.Equal(t, []provider.ToolResultOutput{{Type: provider.ToolOutputErrorText, Text: wantError}}, outputs)
					if entry == "stream" {
						var toolChunks []UIMessageChunk
						for _, chunk := range chunks {
							if chunk.ToolCallID == callID {
								toolChunks = append(toolChunks, chunk)
							}
						}
						require.Len(t, toolChunks, 1)
						assert.Equal(t, ChunkToolInputError, toolChunks[0].Type)
						assert.Equal(t, "b", toolChunks[0].ToolName)
						assert.Equal(t, wantError, toolChunks[0].ErrorText)
						wantInput := json.RawMessage(tc.input)
						if !json.Valid(wantInput) {
							wantInput = json.RawMessage(`"{"`)
						}
						assert.Equal(t, wantInput, toolChunks[0].Input)
					}
				})
			}
		}
	}
}
