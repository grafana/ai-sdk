package aisdk

import (
	"context"
	"errors"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamText_StartStepFromProvider(t *testing.T) {
	for _, tc := range []struct {
		name     string
		warnings []provider.Warning
	}{
		{name: "warnings", warnings: []provider.Warning{{Type: provider.WarnOther, Message: "model warning"}}},
		{name: "empty warnings", warnings: []provider.Warning{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := effectiveToolChoiceModel([]provider.StreamPart{
				{Type: provider.PartStreamStart, Warnings: tc.warnings},
				{Type: provider.PartTextStart, ID: "t1"},
				{Type: provider.PartTextDelta, ID: "t1", Delta: "hi"},
				{Type: provider.PartTextEnd, ID: "t1"},
				{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}},
			})
			result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hi")))
			parts := collectLifecycleParts(result)
			require.NoError(t, result.Err())
			require.Len(t, parts, 7)
			require.IsType(t, StreamStart{}, parts[0])
			start, ok := parts[1].(StreamStartStep)
			require.True(t, ok)
			require.NotNil(t, start.Request)
			assert.JSONEq(t, `{"prompt":"test"}`, string(start.Request.Body))
			assert.Equal(t, tc.warnings, start.Warnings)
			require.Len(t, result.Steps(), 1)
			assert.Equal(t, tc.warnings, result.Steps()[0].Warnings)
			assert.IsType(t, StreamTextStart{}, parts[2])
		})
	}
}

func TestStreamText_OnlyStreamStart(t *testing.T) {
	model := effectiveToolChoiceModel([]provider.StreamPart{{Type: provider.PartStreamStart}})
	result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hi")))
	parts := collectLifecycleParts(result)
	require.Len(t, parts, 2)
	assert.IsType(t, StreamStart{}, parts[0])
	assert.IsType(t, StreamError{}, parts[1])
	assert.ErrorIs(t, result.Err(), ErrNoOutputGenerated)
	assert.Empty(t, result.Steps())
}

func TestStreamText_WithheldToolPartsDoNotStartStep(t *testing.T) {
	parts := []provider.StreamPart{
		{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnOther, Message: "model warning"}}},
		{Type: provider.PartToolInputStart, ID: "call", ToolName: "lookup"},
		{Type: provider.PartToolInputDelta, ID: "call", Delta: `{}`},
		{Type: provider.PartToolInputEnd, ID: "call"},
		{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "lookup", Input: `{}`},
	}
	choice := WithToolChoice(provider.ToolChoice{Type: provider.ToolChoiceNone})
	t.Run("before finish", func(t *testing.T) {
		result := StreamText(t.Context(), effectiveToolChoiceModel(parts),
			WithModelMessages(provider.UserText("hi")), choice)
		got := collectLifecycleParts(result)
		require.Len(t, got, 2)
		assert.IsType(t, StreamStart{}, got[0])
		assert.IsType(t, StreamError{}, got[1])
		assert.ErrorIs(t, result.Err(), ErrNoOutputGenerated)
		assert.Empty(t, result.Steps())
	})
	t.Run("completed violation", func(t *testing.T) {
		usage := provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intPtr(10)},
			OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)},
		}
		model := effectiveToolChoiceModel(append(parts, provider.StreamPart{
			Type: provider.PartFinish, Usage: &usage,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: "tool_calls"},
		}))
		result := StreamText(t.Context(), model, WithModelMessages(provider.UserText("hi")), choice)
		got := collectLifecycleParts(result)
		require.Len(t, got, 5)
		require.IsType(t, StreamStartStep{}, got[1])
		assert.Equal(t, parts[0].Warnings, got[1].(StreamStartStep).Warnings)
		assert.IsType(t, StreamError{}, got[2])
		assert.IsType(t, StreamFinishStep{}, got[3])
		assert.IsType(t, StreamFinish{}, got[4])
		assert.ErrorContains(t, result.Err(), "tool choice was none")
		assert.Equal(t, usage, result.TotalUsage())
		assert.Empty(t, result.ToolCalls())
		assert.Equal(t, provider.FinishReason{Unified: provider.FinishReasonError, Raw: "tool_calls"}, result.FinishReason())
	})
}

func TestStreamText_StartupAbort(t *testing.T) {
	for _, tc := range []struct {
		name      string
		inPrepare bool
		useAgent  bool
	}{
		{name: "DoStream"},
		{name: "PrepareStep", inPrepare: true},
		{name: "Agent Stream", useAgent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			reason := errors.New("startup canceled")
			model := &mockModel{streamFunc: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				cancel(reason)
				return nil, ctx.Err()
			}}
			var callbacks []string
			options := []StreamOption{
				WithMaxRetries(0),
				OnStepStart(func(OnStepStartState) { callbacks = append(callbacks, "step-start") }),
				OnAbort(func(OnAbortState) { callbacks = append(callbacks, "abort") }),
				OnError(func(error) { callbacks = append(callbacks, "error") }),
				OnFinish(func(OnFinishState) { callbacks = append(callbacks, "finish") }),
			}
			wantCallbacks := []string{"step-start", "abort"}
			wantCalls := 1
			if tc.inPrepare {
				options = append(options, WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) {
					cancel(reason)
					return nil, ctx.Err()
				}))
				wantCallbacks = []string{"abort"}
				wantCalls = 0
			}
			var result *StreamTextResult
			if tc.useAgent {
				agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(options...))
				result = agent.Stream(ctx, WithAgentPrompt("hi"))
			} else {
				result = StreamText(ctx, model, append(options, WithModelMessages(provider.UserText("hi")))...)
			}
			assert.Equal(t, []TextStreamPart{StreamStart{}, StreamAbort{Reason: reason.Error()}}, collectLifecycleParts(result))
			assert.Equal(t, wantCallbacks, callbacks)
			assert.Equal(t, wantCalls, model.callCount)
			assert.Empty(t, result.Steps())
			assert.NoError(t, result.Err())
		})
	}
}

func TestGenerateText_StartupAbort(t *testing.T) {
	for _, tc := range []struct {
		name      string
		inPrepare bool
	}{
		{name: "DoStream"},
		{name: "PrepareStep", inPrepare: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, surface := range []string{"GenerateText", "Agent Generate"} {
				t.Run(surface, func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					model := &mockModel{streamFunc: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
						cancel()
						return nil, ctx.Err()
					}}
					var callbackErrors []error
					onError := OnError(func(err error) { callbackErrors = append(callbackErrors, err) })
					prepare := WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) {
						if tc.inPrepare {
							cancel()
							return nil, ctx.Err()
						}
						return nil, nil
					})
					var result *GenerateTextResult
					var err error
					if surface == "Agent Generate" {
						agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(onError, prepare, WithMaxRetries(0)))
						result, err = agent.Generate(ctx, WithAgentPrompt("hi"))
					} else {
						result, err = GenerateText(ctx, model, WithModelMessages(provider.UserText("hi")), onError, prepare, WithMaxRetries(0))
					}
					assert.Nil(t, result)
					assert.ErrorIs(t, err, context.Canceled)
					assert.Empty(t, callbackErrors)
				})
			}
		})
	}
}

func collectLifecycleParts(result *StreamTextResult) []TextStreamPart {
	var parts []TextStreamPart
	for part := range result.FullStream() {
		parts = append(parts, part)
	}
	return parts
}
