package aisdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ Agent = (*ToolLoopAgent)(nil)

type panicModel struct{}

func (panicModel) SpecificationVersion() string               { return "v4" }
func (panicModel) Provider() string                           { return "panic" }
func (panicModel) ModelID() string                            { return "panic-1" }
func (panicModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (panicModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	panic("DoStream called during construction")
}
func (panicModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	panic("DoGenerate called during construction")
}

func TestToolLoopAgent_ConstructionAndIdentity(t *testing.T) {
	tools := ToolSet{
		"search":    {Description: "Search"},
		"summarize": {Description: "Summarize"},
	}

	agent := NewToolLoopAgent(panicModel{},
		WithToolLoopAgentID("research-agent"),
		WithToolLoopAgentOptions(WithTools(tools)),
	)

	assert.Equal(t, AgentVersionV1, agent.Version())
	assert.Equal(t, "research-agent", agent.ID())
	assert.Equal(t, tools, agent.Tools())

	returned := agent.Tools()
	returned["extra"] = Tool{Description: "mutation"}
	assert.NotContains(t, agent.Tools(), "extra")
}

func TestToolLoopAgent_StreamAndGenerateDelegateToExistingResults(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		require.Len(t, opts.Prompt, 1)
		return &provider.StreamResult{Stream: textStreamParts("hello")}, nil
	}}
	agent := NewToolLoopAgent(model)

	streamResult := agent.Stream(context.Background(), WithAgentPrompt("hi"))
	var streamParts []TextStreamPart
	for part := range streamResult.FullStream() {
		streamParts = append(streamParts, part)
	}
	require.NoError(t, streamResult.Err())
	assert.Equal(t, "hello", streamResult.Text())
	assert.NotEmpty(t, streamParts)

	generateResult, err := agent.Generate(context.Background(), WithAgentPrompt("hi"))
	require.NoError(t, err)
	assert.Equal(t, "hello", generateResult.Text)
	assert.Equal(t, provider.FinishReasonStop, generateResult.FinishReason.Unified)
	assert.Len(t, generateResult.Steps, 1)
}

func TestToolLoopAgent_PerCallZeroTimeoutClearsReusableTimeout(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: stallingStreamParts(100 * time.Millisecond)}, nil
	}}
	agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(
		WithTimeout(TimeoutConfig{Total: 50 * time.Millisecond}),
		WithMaxRetries(0),
	))

	result, err := agent.Generate(context.Background(),
		WithAgentPrompt("hi"),
		WithAgentOptions(WithTimeout(TimeoutConfig{})),
	)
	require.NoError(t, err)
	assert.Equal(t, "xx", result.Text)
}

func TestToolLoopAgent_GenerateIgnoresReusableStreamTimeouts(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: stallingStreamParts(200 * time.Millisecond)}, nil
	}}
	agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(
		WithTimeout(TimeoutConfig{FirstChunk: 50 * time.Millisecond, Chunk: 50 * time.Millisecond}),
		WithMaxRetries(0),
	))

	result, err := agent.Generate(context.Background(), WithAgentPrompt("hi"))
	require.NoError(t, err)
	assert.Equal(t, "xx", result.Text)
	assert.Equal(t, []provider.Warning{
		{
			Type:    provider.WarnUnsupported,
			Feature: "timeout.firstChunkMs",
			Details: "The firstChunkMs timeout is only supported by streaming functions.",
		},
		{
			Type:    provider.WarnUnsupported,
			Feature: "timeout.chunkMs",
			Details: "The chunkMs timeout is only supported by streaming functions.",
		},
	}, result.Warnings)
}

func TestToolLoopAgent_GenerateIgnoresReusableStreamOnlyOptions(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		assert.False(t, opts.IncludeRawChunks)
		return &provider.StreamResult{Stream: textStreamParts("hello")}, nil
	}}
	chunkCalls := 0
	agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(
		WithIncludeRawChunks(),
		OnChunk(func(OnChunkState) { chunkCalls++ }),
	))

	result, err := agent.Generate(context.Background(), WithAgentPrompt("hi"))
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Text)
	assert.Equal(t, 0, chunkCalls)
}

func TestToolLoopAgent_SettingsAreFrozenAtConstruction(t *testing.T) {
	tools := ToolSet{"search": {Description: "Search"}}
	headers := map[string]string{"X-Agent": "original"}
	var got provider.CallOptions
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		got = opts
		return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
	}}
	agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(tools), WithHeaders(headers)))

	tools["other"] = Tool{Description: "mutated"}
	headers["X-Agent"] = "mutated"
	headers["X-New"] = "mutated"

	result := agent.Stream(context.Background(), WithAgentPrompt("hi"))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())

	assert.Len(t, got.Tools, 1)
	assert.Equal(t, "search", got.Tools[0].Name)
	assert.Equal(t, "original", got.Headers["X-Agent"])
	assert.NotContains(t, got.Headers, "X-New")
	assert.NotContains(t, agent.Tools(), "other")
}

func TestToolLoopAgent_SettingsMergeDefaultsAndHeaders(t *testing.T) {
	t.Run("settings per call override and immutability", func(t *testing.T) {
		var calls []provider.CallOptions
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			calls = append(calls, opts)
			return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
		}}
		auto := provider.ToolChoice{Type: provider.ToolChoiceAuto}
		none := provider.ToolChoice{Type: provider.ToolChoiceNone}
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(
			WithInstructions("be useful"),
			WithTools(ToolSet{"search": {Description: "Search"}}),
			WithToolChoice(auto),
			WithHeaders(map[string]string{"X-Agent": "settings", "User-Agent": "caller/1"}),
		))

		result := agent.Stream(context.Background(),
			WithAgentPrompt("override"),
			WithAgentOptions(
				WithToolChoice(none),
				WithHeaders(map[string]string{"X-Call": "one"}),
			),
		)
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())

		result = agent.Stream(context.Background(), WithAgentPrompt("second"))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())

		require.Len(t, calls, 2)
		require.NotNil(t, calls[0].ToolChoice)
		assert.Equal(t, provider.ToolChoiceNone, calls[0].ToolChoice.Type)
		assert.Equal(t, "settings", calls[0].Headers["X-Agent"])
		assert.Equal(t, "one", calls[0].Headers["X-Call"])
		assert.Contains(t, calls[0].Headers["User-Agent"], toolLoopAgentUserAgent)
		assert.Len(t, calls[0].Tools, 1)
		assert.Equal(t, provider.RoleSystem, calls[0].Prompt[0].Role)

		require.NotNil(t, calls[1].ToolChoice)
		assert.Equal(t, provider.ToolChoiceAuto, calls[1].ToolChoice.Type)
		assert.Equal(t, "settings", calls[1].Headers["X-Agent"])
		assert.NotContains(t, calls[1].Headers, "X-Call")
		assert.Equal(t, "caller/1 "+toolLoopAgentUserAgent, calls[1].Headers["User-Agent"])
	})

	t.Run("agent default stop count and overrides", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("loop", `{}`)}, nil
		}}
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(loopTool(t))))
		result := agent.Stream(context.Background(), WithAgentPrompt("loop"))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())
		assert.Equal(t, 20, model.callCount)

		model.callCount = 0
		agent = NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(loopTool(t)), WithStopWhen(StepCountIs(3))))
		result = agent.Stream(context.Background(), WithAgentPrompt("loop"))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())
		assert.Equal(t, 3, model.callCount)

		model.callCount = 0
		result = agent.Stream(context.Background(), WithAgentPrompt("loop"), WithAgentOptions(WithStopWhen(StepCountIs(2))))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())
		assert.Equal(t, 2, model.callCount)
	})

	t.Run("direct stream text default remains one step", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("loop", `{}`)}, nil
		}}
		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("loop")),
			WithTools(loopTool(t)),
		)
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())
		assert.Equal(t, 1, model.callCount)
	})
}

func TestToolLoopAgent_CallbacksAndRuntimeContext(t *testing.T) {
	t.Run("callbacks compose in settings then call order", func(t *testing.T) {
		var order []string
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
		}}
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(
			OnStart(func(OnStartState) { order = append(order, "settings-start") }),
			OnStepFinish(func(OnStepFinishState) { order = append(order, "settings-step") }),
		))
		result := agent.Stream(context.Background(), WithAgentPrompt("hi"), WithAgentOptions(
			OnStart(func(OnStartState) { order = append(order, "call-start") }),
			OnStepEnd(func(OnStepFinishState) { order = append(order, "call-step") }),
		))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())
		assert.Equal(t, []string{"settings-start", "call-start", "settings-step", "call-step"}, order)
	})

	t.Run("tool callbacks inherit concurrent execution", func(t *testing.T) {
		var mu sync.Mutex
		starts := 0
		finishes := 0
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: twoToolCallStreamParts("a", "b")}, nil
		}}
		tools := ToolSet{
			"a": executableTool(t, func(ToolExecutionOptions) {}),
			"b": executableTool(t, func(ToolExecutionOptions) {}),
		}
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(
			WithTools(tools),
			WithStopWhen(StepCountIs(1)),
			OnToolCallStart(func(OnToolCallStartState) { mu.Lock(); starts++; mu.Unlock() }),
			OnToolCallFinish(func(OnToolCallFinishState) { mu.Lock(); finishes++; mu.Unlock() }),
		))
		result := agent.Stream(context.Background(), WithAgentPrompt("tools"), WithAgentOptions(
			OnToolCallStart(func(OnToolCallStartState) { mu.Lock(); starts++; mu.Unlock() }),
			OnToolCallFinish(func(OnToolCallFinishState) { mu.Lock(); finishes++; mu.Unlock() }),
		))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())
		assert.Equal(t, 4, starts)
		assert.Equal(t, 4, finishes)
	})

	t.Run("runtime context merge and prepare step override", func(t *testing.T) {
		var contexts []any
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("ctx", `{}`)}, nil
		}}
		tool := executableTool(t, func(opts ToolExecutionOptions) { contexts = append(contexts, opts.Context) })
		agent := NewToolLoopAgent(model,
			WithToolLoopAgentRuntimeContext("agent-context"),
			WithToolLoopAgentOptions(WithTools(ToolSet{"ctx": tool}), WithStopWhen(StepCountIs(1))),
		)

		result := agent.Stream(context.Background(), WithAgentPrompt("ctx"))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())

		result = agent.Stream(context.Background(), WithAgentPrompt("ctx"), WithAgentRuntimeContext("call-context"))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())

		result = agent.Stream(context.Background(), WithAgentPrompt("ctx"), WithAgentRuntimeContext("call-context"), WithAgentOptions(
			WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) {
				return &PrepareStepResult{Context: "step-context"}, nil
			}),
		))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())

		result = agent.Stream(context.Background(), WithAgentPrompt("ctx"), WithAgentOptions(
			WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) {
				return &PrepareStepResult{ActiveTools: []string{"ctx"}}, nil
			}),
		))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())

		assert.Equal(t, []any{"agent-context", "call-context", "step-context", "agent-context"}, contexts)
	})

	t.Run("per-call active tools override reusable settings", func(t *testing.T) {
		var calls [][]provider.Tool
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			calls = append(calls, opts.Tools)
			return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
		}}
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(
			WithTools(ToolSet{"a": {Description: "a"}, "b": {Description: "b"}}),
			WithActiveTools("b"),
		))

		result := agent.Stream(context.Background(), WithAgentPrompt("subset"), WithAgentOptions(WithActiveTools("a")))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())

		result = agent.Stream(context.Background(), WithAgentPrompt("none"), WithAgentOptions(WithActiveTools()))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())

		require.Len(t, calls, 2)
		require.Len(t, calls[0], 1)
		assert.Equal(t, "a", calls[0][0].Name)
		assert.Empty(t, calls[1])
	})
}

func TestAgentUIStreamHelpers(t *testing.T) {
	t.Run("valid messages convert before streaming and preserve original messages", func(t *testing.T) {
		var captured []provider.Message
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			captured = opts.Prompt
			return &provider.StreamResult{Stream: textStreamParts("hello")}, nil
		}}
		agent := NewToolLoopAgent(model)
		messages := []UIMessage{{ID: "m1", Role: RoleUser, Parts: []Part{TextPart{Text: "hi"}}}}

		stream, err := CreateAgentUIStream(context.Background(), agent, messages, WithUIMessageStreamGenerateID(func() string { return "response-1" }))
		require.NoError(t, err)
		chunks := collectUIChunks(stream)
		require.NotEmpty(t, chunks)
		assert.Equal(t, ChunkStart, chunks[0].Type)
		assert.Equal(t, "response-1", chunks[0].MessageID)
		require.Len(t, captured, 1)
		assert.Equal(t, provider.RoleUser, captured[0].Role)
	})

	t.Run("empty assistant message is accepted", func(t *testing.T) {
		var captured []provider.Message
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			captured = opts.Prompt
			return &provider.StreamResult{Stream: textStreamParts("hello")}, nil
		}}
		agent := NewToolLoopAgent(model)
		messages := []UIMessage{
			{ID: "m1", Role: RoleUser, Parts: []Part{TextPart{Text: "hi"}}},
			{ID: "m2", Role: RoleAssistant, Parts: []Part{}},
		}

		stream, err := CreateAgentUIStream(context.Background(), agent, messages, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, collectUIChunks(stream))
		require.Len(t, captured, 1)
		assert.Equal(t, provider.RoleUser, captured[0].Role)
	})

	t.Run("validation errors before provider stream", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			t.Fatal("provider should not be called")
			return nil, nil
		}}
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(ToolSet{"search": {Description: "Search"}})))
		invalid := []UIMessage{{ID: "m1", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{ToolCallID: "c1", ToolName: "missing", State: ToolStateInputAvailable, Input: json.RawMessage(`{}`)}}}}
		stream, err := CreateAgentUIStream(context.Background(), agent, invalid, nil)
		require.Error(t, err)
		assert.Nil(t, stream)
		assert.Equal(t, 0, model.callCount)
	})

	t.Run("invalid states and missing final fields are rejected", func(t *testing.T) {
		agent := NewToolLoopAgent(&mockModel{}, WithToolLoopAgentOptions(WithTools(ToolSet{"search": {Description: "Search"}})))
		cases := []struct {
			name string
			part ToolInvocationPart
		}{
			{name: "unknown state", part: ToolInvocationPart{ToolCallID: "c1", ToolName: "search", State: ToolInvocationState("unknown")}},
			{name: "missing input for input available", part: ToolInvocationPart{ToolCallID: "c1", ToolName: "search", State: ToolStateInputAvailable}},
			{name: "missing input for output", part: ToolInvocationPart{ToolCallID: "c1", ToolName: "search", State: ToolStateOutputAvailable, Output: json.RawMessage(`{"ok":true}`)}},
			{name: "missing output", part: ToolInvocationPart{ToolCallID: "c1", ToolName: "search", State: ToolStateOutputAvailable, Input: json.RawMessage(`{}`)}},
			{name: "missing error", part: ToolInvocationPart{ToolCallID: "c1", ToolName: "search", State: ToolStateOutputError}},
			{name: "missing approval", part: ToolInvocationPart{ToolCallID: "c1", ToolName: "search", State: ToolStateApprovalResponded, Input: json.RawMessage(`{}`)}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				stream, err := CreateAgentUIStream(context.Background(), agent, []UIMessage{{ID: "m1", Role: RoleAssistant, Parts: []Part{tc.part}}}, nil)
				require.Error(t, err)
				assert.Nil(t, stream)
			})
		}

		stream, err := CreateAgentUIStream(context.Background(), agent, []UIMessage{{ID: "m1", Role: RoleAssistant, Parts: []Part{DynamicToolUIPart{ToolCallID: "c1", ToolName: "dynamic", State: ToolStateOutputDenied}}}}, nil)
		require.Error(t, err)
		assert.Nil(t, stream)
	})

	t.Run("provider executed static tool invocation is accepted without agent tool", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
		}}
		agent := NewToolLoopAgent(model)
		messages := []UIMessage{{ID: "m1", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{ToolCallID: "call-1", ToolName: "provider_tool", State: ToolStateOutputAvailable, Input: json.RawMessage(`{}`), Output: json.RawMessage(`{"ok":true}`), ProviderExecuted: true}}}}
		stream, err := CreateAgentUIStream(context.Background(), agent, messages, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, collectUIChunks(stream))
	})

	t.Run("single persisted final tool invocation is accepted", func(t *testing.T) {
		var captured []provider.Message
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			captured = opts.Prompt
			return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
		}}
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(ToolSet{"search": {Description: "Search"}})))
		messages := []UIMessage{{ID: "m1", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{ToolCallID: "call-1", ToolName: "search", State: ToolStateOutputAvailable, Input: json.RawMessage(`{"q":"x"}`), Output: json.RawMessage(`{"ok":true}`)}}}}
		stream, err := CreateAgentUIStream(context.Background(), agent, messages, nil)
		require.NoError(t, err)
		_ = collectUIChunks(stream)
		require.Len(t, captured, 2)
		assert.Equal(t, provider.RoleAssistant, captured[0].Role)
		assert.Equal(t, provider.RoleTool, captured[1].Role)
	})

	t.Run("http helper writes existing SSE framing", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("hi")}, nil
		}}
		agent := NewToolLoopAgent(model)
		rec := httptest.NewRecorder()
		err := WriteAgentUIStream(rec, context.Background(), agent, []UIMessage{{ID: "m1", Role: RoleUser, Parts: []Part{TextPart{Text: "hi"}}}}, nil)
		require.NoError(t, err)
		assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
		assert.Equal(t, "v1", rec.Header().Get("x-vercel-ai-ui-message-stream"))
		assert.Contains(t, rec.Body.String(), "data: ")
		assert.Contains(t, rec.Body.String(), "data: [DONE]\n\n")
	})
}

func TestToolLoopAgent_InheritedOrchestrationBehavior(t *testing.T) {
	t.Run("pending approval emits request and stops current invocation", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("danger", `{}`)}, nil
		}}
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(ToolSet{"danger": {Description: "Danger", Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		}, NeedsApproval: ApprovalRequired()}})))
		result := agent.Stream(context.Background(), WithAgentPrompt("danger"))
		var sawApproval bool
		for part := range result.FullStream() {
			if _, ok := part.(StreamToolApprovalRequest); ok {
				sawApproval = true
			}
		}
		require.NoError(t, result.Err())
		assert.True(t, sawApproval)
		assert.Equal(t, 1, model.callCount)
	})

	t.Run("approval responses resume through generate", func(t *testing.T) {
		var sawToolMessage bool
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			sawToolMessage = false
			for _, msg := range opts.Prompt {
				if msg.Role == provider.RoleTool {
					sawToolMessage = true
				}
			}
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		}}
		approved := true
		messages := []UIMessage{{ID: "m1", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{ToolCallID: "c1", ToolName: "danger", State: ToolStateApprovalResponded, Input: json.RawMessage(`{}`), Approval: &ToolApproval{ID: "approval-1", Approved: &approved}}}}}
		executed := false
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(ToolSet{"danger": {Description: "Danger", Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executed = true
			return json.RawMessage(`{"ok":true}`), nil
		}}})))
		result, err := agent.Generate(context.Background(), WithAgentMessages(messages...))
		require.NoError(t, err)
		assert.True(t, executed)
		assert.True(t, sawToolMessage)
		assert.Equal(t, "done", result.Text)

		denied := false
		messages[0].Parts = []Part{ToolInvocationPart{ToolCallID: "c1", ToolName: "danger", State: ToolStateApprovalResponded, Input: json.RawMessage(`{}`), Approval: &ToolApproval{ID: "approval-1", Approved: &denied, Reason: "no"}}}
		executed = false
		_, err = agent.Generate(context.Background(), WithAgentMessages(messages...))
		require.NoError(t, err)
		assert.False(t, executed)
		assert.True(t, sawToolMessage)
	})

	t.Run("stream parses output for length finish reason", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 4)
			ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `{"name":"Ada"}`}
			ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonLength}}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		}}
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithOutput(testJSONOutput{})))
		result := agent.Stream(context.Background(), WithAgentPrompt("json"))
		for range result.FullStream() {
		}
		require.NoError(t, result.OutputError())
		assert.Equal(t, map[string]any{"name": "Ada"}, result.OutputValue())
	})

	t.Run("external provider executed structured output and stream error delegation", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("external", `{}`)}, nil
		}}
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(ToolSet{"external": {Description: "External"}})))
		result := agent.Stream(context.Background(), WithAgentPrompt("external"))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())
		assert.Equal(t, 1, model.callCount)

		model = &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: providerExecutedToolResultParts()}, nil
		}}
		agent = NewToolLoopAgent(model)
		result = agent.Stream(context.Background(), WithAgentPrompt("provider tool"))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())
		assert.Equal(t, 1, model.callCount)

		model = &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts(`{"name":"Ada"}`)}, nil
		}}
		agent = NewToolLoopAgent(model, WithToolLoopAgentOptions(WithOutput(testJSONOutput{})))
		gen, err := agent.Generate(context.Background(), WithAgentPrompt("json"))
		require.NoError(t, err)
		require.NoError(t, gen.OutputError)
		assert.Equal(t, map[string]any{"name": "Ada"}, gen.Output)

		apiErr := &provider.APICallError{Message: "boom"}
		model = &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 1)
			ch <- provider.StreamPart{Type: provider.PartError, APICallError: apiErr}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		}}
		agent = NewToolLoopAgent(model)
		result = agent.Stream(context.Background(), WithAgentPrompt("error"))
		for range result.FullStream() {
		}
		require.Error(t, result.Err())
	})
}

func loopTool(t *testing.T) ToolSet {
	t.Helper()
	return ToolSet{"loop": executableTool(t, func(ToolExecutionOptions) {})}
}

func executableTool(t *testing.T, inspect func(ToolExecutionOptions)) Tool {
	t.Helper()
	return Tool{
		Description: "Tool",
		InputSchema: testMustSchema(t, `{"type":"object"}`),
		Execute: func(_ context.Context, _ json.RawMessage, opts ToolExecutionOptions) (json.RawMessage, error) {
			if inspect != nil {
				inspect(opts)
			}
			return json.RawMessage(`{"ok":true}`), nil
		},
	}
}

func twoToolCallStreamParts(first, second string) <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: first, Input: `{}`}
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c2", ToolName: second, Input: `{}`}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
	}()
	return ch
}

func providerExecutedToolResultParts() <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "providerTool", Input: `{}`, ProviderExecuted: true}
		ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "providerTool", ProviderExecuted: true, Result: json.RawMessage(`{"ok":true}`)}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
	}()
	return ch
}

type testJSONOutput struct{}

func (testJSONOutput) ResponseFormat() *provider.ResponseFormat {
	return &provider.ResponseFormat{Type: provider.ResponseFormatJSON}
}

func (testJSONOutput) ParseComplete(text string) (any, error) {
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (testJSONOutput) ParsePartial(text string) (any, bool) { return nil, false }

func collectUIChunks(stream <-chan UIMessageChunk) []UIMessageChunk {
	var chunks []UIMessageChunk
	for chunk := range stream {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func TestCreateAgentUIStream_UsesToolModelOutput(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		require.Len(t, opts.Prompt, 2)
		output := opts.Prompt[1].Content[0].Output
		require.NotNil(t, output)
		assert.Equal(t, provider.ToolOutputText, output.Type)
		assert.Equal(t, "converted", output.Text)
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}
	agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(ToolSet{"weather": {
		ToModelOutput: func(ToolOutputContext) (*provider.ToolResultOutput, error) {
			return &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "converted"}, nil
		},
	}})))
	messages := []UIMessage{{ID: "m1", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{
		ToolCallID: "c1", ToolName: "weather", State: ToolStateOutputAvailable,
		Input: json.RawMessage(`{}`), Output: json.RawMessage(`{"temp":72}`),
	}}}}

	stream, err := CreateAgentUIStream(context.Background(), agent, messages)
	require.NoError(t, err)
	collectUIChunks(stream)
	assert.Equal(t, 1, model.callCount)
}

func TestCreateAgentUIStream_ChunksMatchExistingPath(t *testing.T) {
	agentModel := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: textStreamParts("same")}, nil
	}}
	agent := NewToolLoopAgent(agentModel)
	messages := []UIMessage{{ID: "m1", Role: RoleUser, Parts: []Part{TextPart{Text: "hi"}}}}
	agentStream, err := CreateAgentUIStream(context.Background(), agent, messages, WithUIMessageStreamGenerateID(func() string { return "id" }))
	require.NoError(t, err)
	agentChunks := collectUIChunks(agentStream)

	directModel := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: textStreamParts("same")}, nil
	}}
	direct := StreamText(context.Background(), directModel, WithMessages(messages...))
	directChunks := collectUIChunks(direct.ToUIMessageStream(
		WithUIMessageStreamOriginalMessages(messages...),
		WithUIMessageStreamGenerateID(func() string { return "id" }),
	))

	assert.Equal(t, directChunks, agentChunks)
}

func ExampleToolLoopAgent() {
	_ = NewToolLoopAgent(nil,
		WithToolLoopAgentID("assistant"),
		WithToolLoopAgentOptions(WithInstructions("Be concise.")),
	)
	fmt.Println(AgentVersionV1)
	// Output: agent-v1
}
