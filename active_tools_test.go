package aisdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActiveToolsPreserveLateProviderResult(t *testing.T) {
	t.Run("plain", func(t *testing.T) { checkActiveToolsLateProviderResult(t, nil) })
	t.Run("routed", func(t *testing.T) { checkActiveToolsLateProviderResult(t, ToolRoutes{"hosted": {Direct: true}}) })
}

func checkActiveToolsLateProviderResult(t *testing.T, routes ToolRoutes) {
	t.Helper()
	const hosted = "hosted"
	const callID = "provider-1"
	converted := &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "converted"}
	tools := ToolSet{
		"a": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`"done"`), nil
		}},
		hosted: {Type: UserToolProvider, ID: "test.hosted", ToModelOutput: func(ctx ToolOutputContext) (*provider.ToolResultOutput, error) {
			assert.Nil(t, routes, "an inactive routed tool must not supply the converter")
			assert.Equal(t, callID, ctx.ToolCallID)
			assert.JSONEq(t, `{"late":true}`, string(ctx.Output))
			return converted, nil
		}},
	}
	calls := 0
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		calls++
		if calls == 1 {
			assert.Equal(t, []string{"a", hosted}, toolNames(opts.Tools))
			return &provider.StreamResult{Stream: discoveryParts(
				provider.StreamPart{Type: provider.PartToolCall, ToolCallID: callID, ToolName: hosted, Input: `{}`, ProviderExecuted: true},
				discoveryCall("local-1", "a", `{}`),
			)}, nil
		}
		assert.Equal(t, []string{"a"}, toolNames(opts.Tools))
		return &provider.StreamResult{Stream: discoveryParts(provider.StreamPart{
			Type: provider.PartToolResult, ToolCallID: callID, ToolName: hosted,
			ProviderExecuted: true, Result: json.RawMessage(`{"late":true}`),
		})}, nil
	}}
	result, err := GenerateText(t.Context(), model, WithModelMessages(provider.UserText("go")), WithTools(tools),
		WithToolRoutes(routes), WithStopWhen(StepCountIs(2)), WithPrepareStep(func(state PrepareStepState) (*PrepareStepResult, error) {
			if state.StepNumber == 1 {
				return &PrepareStepResult{ActiveTools: []string{"a"}}, nil
			}
			return nil, nil
		}))
	require.NoError(t, err)
	require.Len(t, result.Steps, 2)
	step := result.Steps[1]
	require.Len(t, step.ToolResults, 1)
	require.Len(t, step.Response.Messages, 1)
	require.Len(t, step.Response.Messages[0].Content, 1)
	output := step.Response.Messages[0].Content[0].Output
	if routes == nil {
		assert.Equal(t, converted, step.ToolResults[0].ModelOutput)
		assert.Equal(t, converted, output)
	} else {
		assert.Nil(t, step.ToolResults[0].ModelOutput)
		assert.Equal(t, &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"late":true}`)}, output)
	}
}

func TestActiveToolsRejectInactiveCall_StreamingInputCallbacks(t *testing.T) {
	var starts, deltas []string
	streamedTool := Tool{
		OnInputStart: func(opts ToolExecutionOptions) {
			starts = append(starts, opts.ToolCallID)
		},
		OnInputDelta: func(delta string, opts ToolExecutionOptions) {
			deltas = append(deltas, opts.ToolCallID+":"+delta)
		},
	}
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		assert.Equal(t, []string{"a"}, toolNames(opts.Tools))
		var parts []provider.StreamPart
		for _, name := range []string{"a", "b"} {
			parts = append(parts,
				provider.StreamPart{Type: provider.PartToolInputStart, ID: name, ToolName: name},
				provider.StreamPart{Type: provider.PartToolInputDelta, ID: name, Delta: `{"value":`},
				provider.StreamPart{Type: provider.PartToolInputDelta, ID: name, Delta: `1}`},
				provider.StreamPart{Type: provider.PartToolInputEnd, ID: name},
				discoveryCall(name, name, `{"value":1}`),
			)
		}
		return &provider.StreamResult{Stream: discoveryParts(parts...)}, nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("go")),
		WithTools(ToolSet{"a": streamedTool, "b": streamedTool}), WithActiveTools("a"))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	assert.Equal(t, []string{"a"}, starts)
	assert.Equal(t, []string{`a:{"value":`, `a:1}`}, deltas)
}

func TestActiveToolsRejectInactiveCall(t *testing.T) {
	for _, tc := range []struct {
		name      string
		active    []string
		prepare   PrepareStepFunc
		input     string
		wantFirst []string
		wantNext  []string
		available string
	}{
		{"global", []string{"a"}, nil, `{}`, []string{"a"}, []string{"a"}, "a"},
		{"malformed input", []string{"a"}, nil, `{`, []string{"a"}, []string{"a"}, "a"},
		{"whitespace input", []string{"a"}, nil, " \t", []string{"a"}, []string{"a"}, "a"},
		{"step override", []string{"a", "b"}, func(p PrepareStepState) (*PrepareStepResult, error) {
			if p.StepNumber == 0 {
				return &PrepareStepResult{ActiveTools: []string{"a"}}, nil
			}
			return nil, nil
		}, `{}`, []string{"a"}, []string{"a", "b"}, "a"},
		{"step empty", []string{"a"}, func(p PrepareStepState) (*PrepareStepResult, error) {
			if p.StepNumber == 0 {
				return &PrepareStepResult{ActiveTools: []string{}}, nil
			}
			return &PrepareStepResult{}, nil
		}, `{}`, nil, []string{"a"}, ""},
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
							if len(tc.wantFirst) == 0 {
								assert.Equal(t, setup.routes == nil, opts.Tools == nil)
							}
							return &provider.StreamResult{Stream: toolCallStreamParts("b", tc.input)}, nil
						}
						assert.Equal(t, tc.wantNext, toolNames(opts.Tools))
						nextPrompt = opts.Prompt
						return &provider.StreamResult{Stream: textStreamParts("recovered")}, nil
					}}

					ctx := t.Context()
					messages := WithModelMessages(provider.UserText("go"))
					registry, routes, stop := WithTools(tools), WithToolRoutes(setup.routes), WithStopWhen(StepCountIs(2))
					active, prepare := WithActiveTools(tc.active...), WithPrepareStep(tc.prepare)
					var steps []StepResult
					var chunks []UIMessageChunk
					switch entry {
					case "stream":
						result := StreamText(ctx, model, messages, registry, routes, stop, active, prepare)
						chunks = collectUIChunks(result.ToUIMessageStream())
						require.NoError(t, result.Err())
						steps = result.Steps()
					case "generate":
						result, err := GenerateText(ctx, model, messages, registry, routes, stop, active, prepare)
						require.NoError(t, err)
						steps = result.Steps
					case "agent":
						agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(registry, routes, stop, active, prepare))
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
					var inputs []json.RawMessage
					var outputs []provider.ToolResultOutput
					for _, message := range nextPrompt {
						for _, part := range message.Content {
							if part.ToolCallID != callID {
								continue
							}
							if message.Role == provider.RoleAssistant && part.Type == provider.ContentPartTypeToolCall {
								inputs = append(inputs, part.Input)
							}
							if part.Type == provider.ContentPartTypeToolResult && part.Output != nil {
								outputs = append(outputs, *part.Output)
							}
						}
					}
					assert.Equal(t, []json.RawMessage{json.RawMessage(`{}`)}, inputs)
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
							wantInput, _ = json.Marshal(tc.input)
						}
						assert.Equal(t, wantInput, toolChunks[0].Input)
					}
				})
			}
		}
	}
}
