package aisdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolCallers_Validation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		routes ToolRoutes
		want   string
	}{
		{name: "unknown tool", routes: ToolRoutes{"missing": {Direct: true}}, want: "unknown tool"},
		{name: "non caller", routes: ToolRoutes{"lookup": {Callers: []string{"other"}}}, want: "invalid caller"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: textStreamParts("unexpected")}, nil
			}}
			result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")),
				WithTools(ToolSet{"lookup": {}, "other": {}}), WithToolRoutes(tc.routes))
			for range result.FullStream() {
			}
			require.ErrorContains(t, result.Err(), tc.want)
			assert.Zero(t, model.callCount)
		})
	}
}

func TestToolCallers_AbsentAndEmptyTools(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tools   ToolSet
		wantNil bool
	}{
		{name: "absent", wantNil: true},
		{name: "explicitly empty", tools: ToolSet{}, wantNil: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []provider.Tool
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				got = opts.Tools
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")),
				WithTools(tc.tools), WithToolRoutes(ToolRoutes{}))
			for range result.FullStream() {
			}
			require.NoError(t, result.Err())
			if tc.wantNil {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
				assert.Empty(t, got)
			}
		})
	}
}

func TestToolCallers_InvalidUnusedLocalCaller(t *testing.T) {
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: textStreamParts("unexpected")}, nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")),
		WithTools(ToolSet{"unused": {Caller: &ToolCaller{Type: ToolCallerLocal}}}), WithToolRoutes(ToolRoutes{}))
	for range result.FullStream() {
	}
	require.ErrorContains(t, result.Err(), "invalid local caller")
	assert.Zero(t, model.callCount)
}

func TestToolCallers_BoundToolDualExecutionRejected(t *testing.T) {
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: textStreamParts("unexpected")}, nil
	}}
	both := Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
		return nil, nil
	}, ExecuteStream: func(context.Context, json.RawMessage, ToolExecutionOptions, func(json.RawMessage) error) error {
		return nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")),
		WithTools(ToolSet{"runner": {Caller: &ToolCaller{Type: ToolCallerLocal, Bind: func(ToolSet) Tool { return both }}}}),
		WithToolRoutes(ToolRoutes{}))
	for range result.FullStream() {
	}
	require.ErrorContains(t, result.Err(), "both Execute and ExecuteStream")
	assert.Zero(t, model.callCount)
}

func TestToolCallers_LocalRouting(t *testing.T) {
	var bound []string
	caller := Tool{Description: "original", Caller: &ToolCaller{
		Type: ToolCallerLocal,
		Bind: func(tools ToolSet) Tool {
			for name := range tools {
				bound = append(bound, name)
			}
			return Tool{Description: "bound", Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				return json.Marshal(bound)
			}}
		},
	}}
	tools := ToolSet{
		"code_mode": caller,
		"getInventory": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`{"units":42}`), nil
		}},
		"unrelated": {},
	}
	var modelTools []provider.Tool
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		modelTools = opts.Tools
		return &provider.StreamResult{Stream: toolCallStreamParts("code_mode", `{}`)}, nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")),
		WithTools(tools), WithToolRoutes(ToolRoutes{"getInventory": {Callers: []string{"code_mode"}}}))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	assert.Equal(t, []string{"getInventory"}, bound)
	assert.Equal(t, []string{"code_mode", "unrelated"}, toolNames(modelTools))
	assert.Equal(t, "bound", modelTools[0].Description)
	require.Len(t, result.ToolResults(), 1)
	assert.JSONEq(t, `["getInventory"]`, string(result.ToolResults()[0].Output))
}

func TestToolCallers_ModelVisibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		route  ToolRoute
		active []string
		want   []string
	}{
		{name: "direct and local", route: ToolRoute{Direct: true, Callers: []string{"code_mode"}}, want: []string{"code_mode", "lookup"}},
		{name: "direct only", route: ToolRoute{Direct: true}, want: []string{"code_mode", "lookup"}},
		{name: "local only", route: ToolRoute{Callers: []string{"code_mode"}}, want: []string{"code_mode"}},
		{name: "zero route", want: []string{"code_mode"}},
		{name: "inactive caller", route: ToolRoute{Callers: []string{"code_mode"}}, active: []string{"lookup"}, want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				assert.Equal(t, tc.want, toolNames(opts.Tools))
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			opts := []StreamOption{WithModelMessages(provider.UserText("hello")),
				WithTools(ToolSet{"code_mode": {Caller: &ToolCaller{Type: ToolCallerLocal, Bind: func(ToolSet) Tool { return Tool{} }}}, "lookup": {}}),
				WithToolRoutes(ToolRoutes{"lookup": tc.route})}
			if tc.active != nil {
				opts = append(opts, WithActiveTools(tc.active...))
			}
			result := StreamText(t.Context(), model, opts...)
			for range result.FullStream() {
			}
			require.NoError(t, result.Err())
		})
	}
}

func TestToolCallers_ModelMessageAndActiveTools(t *testing.T) {
	var bound []string
	caller := Tool{Description: "stable", Caller: &ToolCaller{
		Type: ToolCallerLocal,
		Bind: func(tools ToolSet) Tool {
			for name := range tools {
				bound = append(bound, name)
			}
			return Tool{Description: "bound"}
		},
		PrepareModelMessage: func(tools ToolSet) *string {
			message := "caller has inventory"
			return &message
		},
	}}
	var calls []provider.CallOptions
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		calls = append(calls, opts)
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}
	result := StreamText(t.Context(), model,
		WithModelMessages(provider.UserText("caller has inventory")),
		WithTools(ToolSet{"code_mode": caller, "inventory": {}, "hidden": {}}),
		WithToolRoutes(ToolRoutes{"inventory": {Callers: []string{"code_mode"}}, "hidden": {Callers: []string{"code_mode"}}}),
		WithActiveTools("code_mode", "inventory"))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	assert.Equal(t, []string{"inventory"}, bound)
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"code_mode"}, toolNames(calls[0].Tools))
	assert.Equal(t, "stable", calls[0].Tools[0].Description)
	assert.Len(t, calls[0].Prompt, 1)
}

func TestToolCallers_ProviderOptions(t *testing.T) {
	original := provider.ProviderOptions{"test": provider.RawProviderOption{Key: "test", Raw: json.RawMessage(`{"allowedCallers":["manual"]}`)}}
	providerCaller := Tool{Caller: &ToolCaller{Type: ToolCallerProvider, PrepareProviderOptions: func(options provider.ProviderOptions) provider.ProviderOptions {
		assert.JSONEq(t, `{"test":{"allowedCallers":["manual"]}}`, marshalJSON(t, options))
		return provider.ProviderOptions{"test": provider.RawProviderOption{Key: "test", Raw: json.RawMessage(`{"allowedCallers":["programmatic"]}`)}}
	}}}
	var got []provider.Tool
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		got = opts.Tools
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}
	tools := ToolSet{"programmatic": providerCaller, "lookup": {ProviderOptions: original}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")), WithTools(tools),
		WithToolRoutes(ToolRoutes{"lookup": {Direct: true, Callers: []string{"programmatic"}}}))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	assert.Equal(t, []string{"lookup", "programmatic"}, toolNames(got))
	assert.JSONEq(t, `{"test":{"allowedCallers":["programmatic"]}}`, marshalJSON(t, got[0].ProviderOptions))
	assert.JSONEq(t, `{"test":{"allowedCallers":["manual"]}}`, marshalJSON(t, original))
}

func TestToolCallers_ProviderOnlyAndNamedChoice(t *testing.T) {
	choice := provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		assert.Equal(t, []string{"lookup", "programmatic"}, toolNames(opts.Tools))
		assert.Equal(t, &choice, opts.ToolChoice)
		assert.JSONEq(t, `{"test":{"allowedCallers":["programmatic"]}}`, marshalJSON(t, opts.Tools[0].ProviderOptions))
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}
	tools := ToolSet{"programmatic": {Caller: &ToolCaller{Type: ToolCallerProvider, PrepareProviderOptions: func(provider.ProviderOptions) provider.ProviderOptions {
		return provider.ProviderOptions{"test": provider.RawProviderOption{Key: "test", Raw: json.RawMessage(`{"allowedCallers":["programmatic"]}`)}}
	}}}, "lookup": {}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")), WithTools(tools),
		WithToolChoice(choice), WithToolRoutes(ToolRoutes{"lookup": {Callers: []string{"programmatic"}}}))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
}

func TestToolCallers_PerStepActiveTools(t *testing.T) {
	var seen [][]string
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		seen = append(seen, toolNames(opts.Tools))
		if len(seen) == 1 {
			return &provider.StreamResult{Stream: toolCallStreamParts("runner", `{}`)}, nil
		}
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")), WithTools(ToolSet{
		"runner": {Caller: &ToolCaller{Type: ToolCallerLocal, Bind: func(ToolSet) Tool {
			return Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				return json.RawMessage(`"ok"`), nil
			}}
		}}}, "lookup": {},
	}), WithToolRoutes(ToolRoutes{"lookup": {Callers: []string{"runner"}}}), WithStopWhen(StepCountIs(2)),
		WithPrepareStep(func(state PrepareStepState) (*PrepareStepResult, error) {
			if state.StepNumber == 1 {
				return &PrepareStepResult{ActiveTools: []string{}}, nil
			}
			return nil, nil
		}))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	assert.Equal(t, [][]string{{"runner"}, nil}, seen)
}

func TestToolCallers_EntryPointsAndProviderCalls(t *testing.T) {
	tools := ToolSet{"hidden": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
		return json.RawMessage(`"ok"`), nil
	}}, "runner": {Caller: &ToolCaller{Type: ToolCallerLocal, Bind: func(ToolSet) Tool { return Tool{} }}}}
	routes := ToolRoutes{"hidden": {Callers: []string{"runner"}}}
	for _, tc := range []struct {
		name string
		run  func(*mockModel) error
	}{
		{name: "generate", run: func(m *mockModel) error {
			_, err := GenerateText(t.Context(), m, WithModelMessages(provider.UserText("hello")), WithTools(tools), WithToolRoutes(routes))
			return err
		}},
		{name: "agent", run: func(m *mockModel) error {
			agent := NewToolLoopAgent(m, WithToolLoopAgentOptions(WithTools(tools), WithToolRoutes(routes)))
			result := agent.Stream(t.Context(), WithAgentPrompt("hello"))
			for range result.FullStream() {
			}
			return result.Err()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				assert.Equal(t, []string{"runner"}, toolNames(opts.Tools))
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			require.NoError(t, tc.run(model))
		})
	}
}

func TestToolCallers_AgentRoutesSnapshot(t *testing.T) {
	routes := ToolRoutes{"lookup": {Callers: []string{"runner"}}}
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		assert.Equal(t, []string{"runner"}, toolNames(opts.Tools))
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}
	agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(ToolSet{
		"lookup": {},
		"runner": {Caller: &ToolCaller{Type: ToolCallerLocal, Bind: func(ToolSet) Tool { return Tool{} }}},
	}), WithToolRoutes(routes)))
	routes["lookup"].Callers[0] = "unknown"
	result := agent.Stream(t.Context(), WithAgentPrompt("hello"))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
}

func TestToolCallers_ProviderExecutedAndUnknownDynamic(t *testing.T) {
	var executed bool
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 3)
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "lookup", Input: `{}`, ProviderExecuted: true}
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c2", ToolName: "unknown", Input: `{}`, ProviderExecuted: true}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")),
		WithTools(ToolSet{"lookup": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executed = true
			return nil, nil
		}}, "runner": {Caller: &ToolCaller{Type: ToolCallerLocal, Bind: func(ToolSet) Tool { return Tool{} }}}}),
		WithToolRoutes(ToolRoutes{"lookup": {Callers: []string{"runner"}}}))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	assert.False(t, executed)
}

func TestToolCallers_ManualOptionsWithoutRouting(t *testing.T) {
	manual := provider.ProviderOptions{"test": provider.RawProviderOption{Key: "test", Raw: json.RawMessage(`{"allowedCallers":["direct"]}`)}}
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		require.Len(t, opts.Tools, 1)
		assert.JSONEq(t, `{"test":{"allowedCallers":["direct"]}}`, marshalJSON(t, opts.Tools[0].ProviderOptions))
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hello")),
		WithTools(ToolSet{"lookup": {ProviderOptions: manual}}))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
}

func toolNames(tools []provider.Tool) []string {
	if len(tools) == 0 {
		return nil
	}
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

func marshalJSON(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	require.NoError(t, err)
	return string(encoded)
}
