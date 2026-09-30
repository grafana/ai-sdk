package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
