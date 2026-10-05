package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
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

func TestStreamText_EffectiveToolChoice(t *testing.T) {
	required := provider.ToolChoice{Type: provider.ToolChoiceRequired}
	named := provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}
	call := func(id, name string) provider.StreamPart {
		return provider.StreamPart{Type: provider.PartToolCall, ToolCallID: id, ToolName: name, Input: `{}`}
	}
	invalid := call("c1", "lookup")
	invalid.Input = `not JSON`
	hosted := call("c1", "lookup")
	hosted.ProviderExecuted = true
	otherHosted := call("c1", "other")
	otherHosted.ProviderExecuted = true
	resolved := []provider.StreamPart{call("c1", "other"), {Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "other", Result: json.RawMessage(`"hosted"`)}}
	for _, tc := range []struct {
		name       string
		choice     provider.ToolChoice
		calls      []provider.StreamPart
		finish     provider.UnifiedFinishReason
		opts       []StreamOption
		wantError  bool
		executions int
	}{
		{name: "required valid", choice: required, calls: []provider.StreamPart{call("c1", "lookup")}, executions: 1},
		{name: "named valid", choice: named, calls: []provider.StreamPart{call("c1", "lookup")}, executions: 1},
		{name: "named mixed", choice: named, calls: []provider.StreamPart{call("c1", "lookup"), call("c2", "other")}, executions: 2},
		{name: "required invalid presence", choice: required, calls: []provider.StreamPart{invalid}},
		{name: "named invalid presence", choice: named, calls: []provider.StreamPart{invalid}},
		{name: "required provider presence", choice: required, calls: []provider.StreamPart{otherHosted}},
		{name: "named provider presence", choice: named, calls: []provider.StreamPart{hosted}},
		{name: "provider-only miss", choice: named, calls: []provider.StreamPart{otherHosted}, opts: []StreamOption{WithStopWhen(StepCountIs(3))}, wantError: true},
		{name: "resolved miss", choice: named, calls: resolved, opts: []StreamOption{WithStopWhen(StepCountIs(3))}, wantError: true},
		{name: "auto no call", choice: provider.ToolChoice{Type: provider.ToolChoiceAuto}},
		{name: "none no call", choice: provider.ToolChoice{Type: provider.ToolChoiceNone}},
		{name: "none returned call", choice: provider.ToolChoice{Type: provider.ToolChoiceNone}, calls: []provider.StreamPart{call("c1", "other")}, executions: 1},
		{name: "absent tools", choice: required, opts: []StreamOption{WithTools(nil)}, wantError: true},
		{name: "empty tools", choice: named, opts: []StreamOption{WithTools(ToolSet{})}, wantError: true},
		{name: "filtered empty tools", choice: named, opts: []StreamOption{WithActiveTools()}, wantError: true},
		{name: "length with call", choice: named, calls: []provider.StreamPart{call("c1", "lookup")}, finish: provider.FinishReasonLength},
		{name: "error finish without call", choice: required, finish: provider.FinishReasonError, wantError: true},
		{name: "duplicate selected overwritten", choice: named, calls: []provider.StreamPart{call("c1", "lookup"), call("c1", "other")}, wantError: true},
		{name: "duplicate selected last", choice: named, calls: []provider.StreamPart{call("c1", "other"), call("c1", "lookup")}, finish: provider.FinishReasonLength},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := effectiveToolChoiceParts(tc.calls...)
			if tc.finish != "" {
				parts[len(parts)-1].FinishReason.Unified = tc.finish
			}
			model := effectiveToolChoiceModel(parts)
			var executions atomic.Int32
			execute := func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				executions.Add(1)
				return json.RawMessage(`"ok"`), nil
			}
			opts := []StreamOption{WithModelMessages(provider.UserText("test")), WithToolChoice(tc.choice), WithTools(ToolSet{"lookup": {Execute: execute}, "other": {Execute: execute}})}
			result := StreamText(t.Context(), model, append(opts, tc.opts...)...)
			for range result.FullStream() {
			}
			if tc.wantError {
				require.ErrorContains(t, result.Err(), "tool choice")
			} else {
				require.NoError(t, result.Err())
			}
			assert.EqualValues(t, tc.executions, executions.Load())
			assert.Equal(t, 1, model.callCount)
			require.Len(t, result.Steps(), 1)
		})
	}
	t.Run("serialized call text", func(t *testing.T) {
		model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts(`{"toolName":"lookup","input":{}}`)}, nil
		}}
		result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("test")), WithToolChoice(required), WithTools(ToolSet{"lookup": {}}))
		for range result.FullStream() {
		}
		require.ErrorContains(t, result.Err(), "tool choice")
		assert.Equal(t, `{"toolName":"lookup","input":{}}`, result.Text())
	})
}

func TestStreamText_EffectiveToolChoiceStepOverride(t *testing.T) {
	auto := provider.ToolChoice{Type: provider.ToolChoiceAuto}
	none := provider.ToolChoice{Type: provider.ToolChoiceNone}
	required := provider.ToolChoice{Type: provider.ToolChoiceRequired}
	lookup := provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}
	other := provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "other"}
	for _, tc := range []struct {
		name                 string
		configured, override provider.ToolChoice
		callName             string
		wantError            bool
	}{
		{"auto to required", auto, required, "", true},
		{"auto to named", auto, lookup, "other", true},
		{"required to auto", required, auto, "", false},
		{"required to none", required, none, "", false},
		{"named to auto", lookup, auto, "", false},
		{"named to none", lookup, none, "", false},
		{"changed named miss", lookup, other, "lookup", true},
		{"changed named valid", lookup, other, "other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []provider.StreamPart
			if tc.callName != "" {
				calls = append(calls, provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: tc.callName, Input: `{}`})
			}
			model := effectiveToolChoiceModel(effectiveToolChoiceParts(calls...))
			stream := model.streamFunc
			model.streamFunc = func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				assert.Equal(t, &tc.override, opts.ToolChoice)
				return stream(ctx, opts)
			}
			result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("test")), WithTools(ToolSet{"lookup": {}, "other": {}}), WithToolChoice(tc.configured),
				WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) {
					return &PrepareStepResult{ToolChoice: &tc.override}, nil
				}))
			for range result.FullStream() {
			}
			if tc.wantError {
				require.ErrorContains(t, result.Err(), "tool choice")
			} else {
				require.NoError(t, result.Err())
			}
		})
	}
	for _, configured := range []provider.ToolChoice{auto, required, lookup} {
		t.Run("reset/"+string(configured.Type), func(t *testing.T) {
			model := effectiveToolChoiceModel(effectiveToolChoiceParts())
			stream := model.streamFunc
			var choices []provider.ToolChoice
			model.streamFunc = func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				choices = append(choices, *opts.ToolChoice)
				if len(choices) == 1 {
					return effectiveToolChoiceModel(effectiveToolChoiceParts(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "other", Input: `{}`})).streamFunc(ctx, opts)
				}
				return stream(ctx, opts)
			}
			result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("test")), WithToolChoice(configured), WithStopWhen(StepCountIs(3)),
				WithTools(ToolSet{"lookup": {}, "other": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`"ok"`), nil
				}}}),
				WithPrepareStep(func(state PrepareStepState) (*PrepareStepResult, error) {
					if state.StepNumber == 0 {
						return &PrepareStepResult{ToolChoice: &other}, nil
					}
					return &PrepareStepResult{}, nil
				}))
			for range result.FullStream() {
			}
			assert.Equal(t, []provider.ToolChoice{other, configured}, choices)
			if configured.Type == provider.ToolChoiceAuto {
				require.NoError(t, result.Err())
			} else {
				require.ErrorContains(t, result.Err(), "tool choice")
			}
			require.Len(t, result.Steps(), 2)
			assert.Equal(t, 20, *result.TotalUsage().InputTokens.Total)
			assert.Equal(t, 10, *result.TotalUsage().OutputTokens.Total)
			assert.Equal(t, provider.FinishReasonToolCalls, result.Steps()[0].FinishReason.Unified)
		})
	}
}

func TestStreamText_EffectiveToolChoiceApprovalSuppression(t *testing.T) {
	for _, tc := range []struct {
		name     string
		approval *ToolApprovalConfig
		policy   ToolApprovalStatus
		perTool  bool
	}{
		{name: "static", approval: ApprovalRequired()},
		{name: "dynamic"},
		{name: "generic user", policy: ToolApprovalUserApproval},
		{name: "generic approved", policy: ToolApprovalApproved},
		{name: "generic denied", policy: ToolApprovalDenied},
		{name: "per-tool user", policy: ToolApprovalUserApproval, perTool: true},
		{name: "per-tool approved", policy: ToolApprovalApproved, perTool: true},
		{name: "per-tool denied", policy: ToolApprovalDenied, perTool: true},
	} {
		for _, mode := range []string{"ordinary", "streaming"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				effects, ids := 0, 0
				tool := Tool{NeedsApproval: tc.approval}
				if tc.name == "dynamic" {
					tool.NeedsApproval = ApprovalIf(func(json.RawMessage, ToolExecutionOptions) (bool, error) {
						effects++
						return true, errors.New("unexpected approval evaluation")
					})
				}
				if mode == "streaming" {
					tool.ExecuteStream = func(context.Context, json.RawMessage, ToolExecutionOptions, func(json.RawMessage) error) error {
						effects++
						return errors.New("unexpected execution")
					}
				} else {
					tool.Execute = func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
						effects++
						return nil, errors.New("unexpected execution")
					}
				}
				opts := []StreamOption{WithModelMessages(provider.UserText("test")), WithToolChoice(provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}), WithTools(ToolSet{"lookup": {}, "other": tool}),
					WithToolApprovalSecret("secret"), WithGenerateID(func() string { ids++; return "id" }),
					OnToolCallStart(func(OnToolCallStartState) { effects++ }), OnToolCallFinish(func(OnToolCallFinishState) { effects++ })}
				if tc.policy != "" {
					policy := ToolApprovalFunc(func(ToolApprovalOptions) (ToolApprovalDecision, error) {
						effects++
						return ToolApprovalDecision{Status: tc.policy}, nil
					})
					if tc.perTool {
						opts = append(opts, WithToolApproval(ToolApprovalMap{"other": ApprovalPolicyFunc(func(json.RawMessage, ToolExecutionOptions) (ToolApprovalDecision, error) {
							return policy(ToolApprovalOptions{})
						})}))
					} else {
						opts = append(opts, WithToolApproval(policy))
					}
				}
				result := StreamText(t.Context(), effectiveToolChoiceModel(effectiveToolChoiceParts(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "other", Input: `{}`})), opts...)
				for event := range result.FullStream() {
					switch event.(type) {
					case StreamToolApprovalRequest, StreamToolApprovalResponse, StreamToolOutputDenied, StreamToolResult:
						assert.Failf(t, "unexpected local effect", "%T", event)
					}
				}
				require.ErrorContains(t, result.Err(), "tool choice")
				assert.Zero(t, effects)
				assert.Equal(t, 1, ids)
				require.Len(t, result.Steps(), 1)
				assert.Empty(t, result.Steps()[0].ToolApprovalRequests)
				assert.Empty(t, result.Steps()[0].ToolApprovalResponses)
				assert.Empty(t, result.ToolResults())
			})
		}
	}
	for _, status := range []ToolApprovalStatus{ToolApprovalUserApproval, ToolApprovalApproved, ToolApprovalDenied} {
		t.Run("valid approval/"+string(status), func(t *testing.T) {
			executions := 0
			result := StreamText(t.Context(), effectiveToolChoiceModel(effectiveToolChoiceParts(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "lookup", Input: `{}`})),
				WithModelMessages(provider.UserText("test")), WithToolChoice(provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}),
				WithToolApproval(ToolApprovalMap{"lookup": ApprovalPolicy(status)}),
				WithTools(ToolSet{"lookup": {NeedsApproval: ApprovalRequired(), Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					executions++
					return json.RawMessage(`"ok"`), nil
				}}}))
			for range result.FullStream() {
			}
			require.NoError(t, result.Err())
			require.Len(t, result.Steps(), 1)
			assert.Len(t, result.Steps()[0].ToolApprovalRequests, 1)
			if status == ToolApprovalApproved {
				assert.Equal(t, 1, executions)
			} else {
				assert.Zero(t, executions)
			}
			if status == ToolApprovalUserApproval {
				assert.Empty(t, result.ToolResults())
			} else {
				assert.Len(t, result.ToolResults(), 1)
			}
		})
	}
	t.Run("provider events preserved", func(t *testing.T) {
		result := StreamText(t.Context(), effectiveToolChoiceModel(effectiveToolChoiceParts(
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "other", Input: `{}`, ProviderExecuted: true},
			provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "other", Result: json.RawMessage(`"hosted"`)},
			provider.StreamPart{Type: provider.PartToolApprovalRequest, ToolCallID: "c1", ApprovalID: "provider-approval"},
		)), WithModelMessages(provider.UserText("test")), WithToolChoice(provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}), WithTools(ToolSet{"lookup": {}, "other": {}}), WithToolApprovalSecret("secret"))
		for range result.FullStream() {
		}
		require.ErrorContains(t, result.Err(), "tool choice")
		require.Len(t, result.Steps(), 1)
		step := result.Steps()[0]
		require.Len(t, step.ToolResults, 1)
		assert.True(t, step.ToolResults[0].ProviderExecuted)
		require.Len(t, step.ToolApprovalRequests, 1)
		assert.Equal(t, "provider-approval", step.ToolApprovalRequests[0].ApprovalID)
		assert.NotEmpty(t, step.ToolApprovalRequests[0].Signature)
	})
	t.Run("prior-message approval preserved", func(t *testing.T) {
		executions := 0
		model := effectiveToolChoiceModel(effectiveToolChoiceParts())
		stream := model.streamFunc
		model.streamFunc = func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			assert.Equal(t, 1, executions)
			assert.Equal(t, provider.ContentPartTypeToolResult, opts.Prompt[len(opts.Prompt)-1].Content[0].Type)
			return stream(ctx, opts)
		}
		result := StreamText(t.Context(), model, WithModelMessages(
			provider.NewAssistantMessage(provider.ToolCallPart("c1", "other", json.RawMessage(`{}`)), provider.ToolApprovalRequestPart("apr-1", "c1", false)),
			provider.NewToolMessage(provider.ToolApprovalResponsePart("apr-1", true, "ok")),
		), WithToolChoice(provider.ToolChoice{Type: provider.ToolChoiceRequired}), WithTools(ToolSet{"other": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executions++
			return json.RawMessage(`"ok"`), nil
		}}}))
		for range result.FullStream() {
		}
		require.ErrorContains(t, result.Err(), "tool choice")
		assert.Equal(t, 1, executions)
	})
}

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
