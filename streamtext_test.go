package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testMustSchema(t *testing.T, raw string) schema.Schema {
	t.Helper()
	s, err := schema.SchemaFromJSON(json.RawMessage(raw))
	require.NoError(t, err)
	return s
}

func intPtr(v int) *int { return &v }

type mockModel struct {
	streamFunc func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error)
	callCount  int
}

func (m *mockModel) SpecificationVersion() string               { return "v4" }
func (m *mockModel) Provider() string                           { return "mock" }
func (m *mockModel) ModelID() string                            { return "mock-1" }
func (m *mockModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *mockModel) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	m.callCount++
	return m.streamFunc(ctx, opts)
}
func (m *mockModel) DoGenerate(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}

func textStreamParts(text string) <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: text}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(5)}}}
	}()
	return ch
}

func toolCallStreamParts(toolName, input string) <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: toolName, Input: input}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(3)}}}
	}()
	return ch
}

func finishStreamParts() <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 1)
	ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
	close(ch)
	return ch
}

func TestToolApprovalConfig(t *testing.T) {
	input := json.RawMessage(`{"amount":100}`)

	t.Run("default", func(t *testing.T) {
		needed, err := isApprovalNeeded(Tool{}, input, ToolExecutionOptions{ToolCallID: "c1"})
		require.NoError(t, err)
		assert.False(t, needed)
	})

	t.Run("static", func(t *testing.T) {
		needed, err := isApprovalNeeded(Tool{NeedsApproval: ApprovalRequired()}, input, ToolExecutionOptions{ToolCallID: "c1"})
		require.NoError(t, err)
		assert.True(t, needed)
	})

	t.Run("dynamic true", func(t *testing.T) {
		needed, err := isApprovalNeeded(Tool{NeedsApproval: ApprovalIf(func(got json.RawMessage, opts ToolExecutionOptions) (bool, error) {
			assert.Equal(t, input, got)
			assert.Equal(t, "c1", opts.ToolCallID)
			return true, nil
		})}, input, ToolExecutionOptions{ToolCallID: "c1"})
		require.NoError(t, err)
		assert.True(t, needed)
	})

	t.Run("dynamic false", func(t *testing.T) {
		needed, err := isApprovalNeeded(Tool{NeedsApproval: ApprovalIf(func(json.RawMessage, ToolExecutionOptions) (bool, error) {
			return false, nil
		})}, input, ToolExecutionOptions{ToolCallID: "c1"})
		require.NoError(t, err)
		assert.False(t, needed)
	})

	t.Run("dynamic error", func(t *testing.T) {
		_, err := isApprovalNeeded(Tool{NeedsApproval: ApprovalIf(func(json.RawMessage, ToolExecutionOptions) (bool, error) {
			return false, fmt.Errorf("approval failed")
		})}, input, ToolExecutionOptions{ToolCallID: "c1"})
		require.ErrorContains(t, err, "approval failed")
	})
}

func TestStreamText_PersistedToolOutputUsesModelOutput(t *testing.T) {
	messages := []UIMessage{{ID: "1", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{
		ToolCallID: "c1", ToolName: "weather", State: ToolStateOutputAvailable,
		Input: json.RawMessage(`{"city":"NYC"}`), Output: json.RawMessage(`{"temp":72}`),
	}}}}
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		require.Len(t, opts.Prompt, 2)
		output := opts.Prompt[1].Content[0].Output
		require.NotNil(t, output)
		assert.Equal(t, provider.ToolOutputText, output.Type)
		assert.Equal(t, "72 degrees", output.Text)
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}

	result := StreamText(t.Context(), model,
		WithMessages(messages...),
		WithTools(ToolSet{"weather": {
			ToModelOutput: func(ToolOutputContext) (*provider.ToolResultOutput, error) {
				return &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "72 degrees"}, nil
			},
		}}),
	)
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	assert.Equal(t, 1, model.callCount)
}

func TestStreamText_PersistedToolOutputConversionErrorPreventsModelCall(t *testing.T) {
	expected := fmt.Errorf("cannot convert")
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: textStreamParts("unexpected")}, nil
	}}
	result := StreamText(t.Context(), model,
		WithMessages(UIMessage{ID: "1", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{
			ToolCallID: "c1", ToolName: "weather", State: ToolStateOutputAvailable,
			Input: json.RawMessage(`{}`), Output: json.RawMessage(`{}`),
		}}}),
		WithTools(ToolSet{"weather": {
			ToModelOutput: func(ToolOutputContext) (*provider.ToolResultOutput, error) {
				return nil, expected
			},
		}}),
	)
	for range result.FullStream() {
	}
	require.ErrorIs(t, result.Err(), expected)
	assert.Zero(t, model.callCount)
}

func TestStreamTextSingleStep(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("hello world")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
	)

	var types []string
	for part := range result.FullStream() {
		types = append(types, typeName(part))
	}

	expected := []string{"start", "start-step", "text-start", "text-delta", "text-end", "finish-step", "finish"}
	assert.Equal(t, expected, types)
	assert.Equal(t, "hello world", result.Text())
}

func TestStreamTextPrepareStep_Messages(t *testing.T) {
	initialMessages := []provider.Message{provider.UserText("original")}
	overriddenMessages := []provider.Message{provider.UserText("prepared")}
	var states []PrepareStepState
	var prompts [][]provider.Message

	model := &mockModel{}
	model.streamFunc = func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		prompts = append(prompts, cloneMessages(opts.Prompt))
		if len(prompts) == 1 {
			return &provider.StreamResult{Stream: toolCallStreamParts("weather", `{"city":"NYC"}`)}, nil
		}
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(initialMessages...),
		WithTools(ToolSet{
			"weather": {
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{"temp":72}`), nil
				},
			},
		}),
		WithStopWhen(StepCountIs(3)),
		WithPrepareStep(func(state PrepareStepState) (*PrepareStepResult, error) {
			state.InitialMessages = cloneMessages(state.InitialMessages)
			state.ResponseMessages = cloneMessages(state.ResponseMessages)
			state.Messages = cloneMessages(state.Messages)
			states = append(states, state)
			if len(states) == 1 {
				return &PrepareStepResult{Messages: overriddenMessages}, nil
			}
			return nil, nil
		}),
	)
	for range result.FullStream() {
	}

	require.Len(t, states, 2)
	assert.Equal(t, initialMessages, states[0].InitialMessages)
	assert.Empty(t, states[0].ResponseMessages)
	assert.Equal(t, initialMessages, states[0].Messages)

	assert.Equal(t, initialMessages, states[1].InitialMessages)
	require.Len(t, states[1].ResponseMessages, 2)
	assert.Equal(t, provider.RoleAssistant, states[1].ResponseMessages[0].Role)
	assert.Equal(t, provider.RoleTool, states[1].ResponseMessages[1].Role)
	assert.Equal(t, append(cloneMessages(overriddenMessages), states[1].ResponseMessages...), states[1].Messages)

	require.Len(t, prompts, 2)
	assert.Equal(t, overriddenMessages, prompts[0])
	assert.Equal(t, states[1].Messages, prompts[1])
}

func TestStreamTextPrepareStep_SystemMessages(t *testing.T) {
	initialMessages := []provider.Message{provider.UserText("hello")}
	var state PrepareStepState

	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(initialMessages...),
		WithSystem("you are helpful"),
		WithPrepareStep(func(current PrepareStepState) (*PrepareStepResult, error) {
			state = current
			return nil, nil
		}),
	)
	for range result.FullStream() {
	}

	assert.Equal(t, initialMessages, state.InitialMessages)
	assert.Empty(t, state.ResponseMessages)
	assert.Equal(t, []provider.Message{
		provider.NewSystemMessage("you are helpful"),
		provider.UserText("hello"),
	}, state.Messages)
}

func TestStreamTextPrepareStep_CallSettings(t *testing.T) {
	t.Run("overrides apply to the current step", func(t *testing.T) {
		maxOutputTokens := 1
		temperature := 0.0
		topP := 0.0
		topK := 0
		presencePenalty := 0.0
		frequencyPenalty := 0.0
		seed := 0
		reasoning := provider.ReasoningNone
		var call provider.CallOptions

		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			call = opts
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		}}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("hello")),
			WithMaxOutputTokens(100),
			WithTemperature(0.8),
			WithTopP(0.9),
			WithTopK(40),
			WithPresencePenalty(0.4),
			WithFrequencyPenalty(0.5),
			WithStopSequences("STOP"),
			WithSeed(42),
			WithReasoning(provider.ReasoningHigh),
			WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) {
				return &PrepareStepResult{
					MaxOutputTokens:  &maxOutputTokens,
					Temperature:      &temperature,
					TopP:             &topP,
					TopK:             &topK,
					PresencePenalty:  &presencePenalty,
					FrequencyPenalty: &frequencyPenalty,
					StopSequences:    []string{},
					Seed:             &seed,
					Reasoning:        &reasoning,
				}, nil
			}),
		)
		for range result.FullStream() {
		}

		require.NotNil(t, call.MaxOutputTokens)
		assert.Equal(t, 1, *call.MaxOutputTokens)
		require.NotNil(t, call.Temperature)
		assert.Equal(t, 0.0, *call.Temperature)
		require.NotNil(t, call.TopP)
		assert.Equal(t, 0.0, *call.TopP)
		require.NotNil(t, call.TopK)
		assert.Equal(t, 0, *call.TopK)
		require.NotNil(t, call.PresencePenalty)
		assert.Equal(t, 0.0, *call.PresencePenalty)
		require.NotNil(t, call.FrequencyPenalty)
		assert.Equal(t, 0.0, *call.FrequencyPenalty)
		assert.Empty(t, call.StopSequences)
		require.NotNil(t, call.Seed)
		assert.Equal(t, 0, *call.Seed)
		require.NotNil(t, call.Reasoning)
		assert.Equal(t, provider.ReasoningNone, *call.Reasoning)
	})

	t.Run("invalid max output tokens stops before model call", func(t *testing.T) {
		calls := 0
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			calls++
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		}}
		maxOutputTokens := 0

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("hello")),
			WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) {
				return &PrepareStepResult{MaxOutputTokens: &maxOutputTokens}, nil
			}),
		)
		for range result.FullStream() {
		}

		assert.Zero(t, calls)
		require.Error(t, result.Err())
		assert.ErrorContains(t, result.Err(), "maxOutputTokens must be >= 1")
	})

	t.Run("undefined settings fall back to outer settings", func(t *testing.T) {
		var call provider.CallOptions
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			call = opts
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		}}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("hello")),
			WithTemperature(0.7),
			WithStopSequences("STOP"),
			WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) {
				return &PrepareStepResult{}, nil
			}),
		)
		for range result.FullStream() {
		}

		require.NotNil(t, call.Temperature)
		assert.Equal(t, 0.7, *call.Temperature)
		assert.Equal(t, []string{"STOP"}, call.StopSequences)
	})

	t.Run("overrides do not carry to later steps", func(t *testing.T) {
		var temperatures []float64
		callCount := 0
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			require.NotNil(t, opts.Temperature)
			temperatures = append(temperatures, *opts.Temperature)
			callCount++
			if callCount < 3 {
				return &provider.StreamResult{Stream: toolCallStreamParts("lookup", `{}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		}}
		override := 0.0

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("hello")),
			WithTemperature(0.7),
			WithTools(ToolSet{"lookup": {
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{"ok":true}`), nil
				},
			}}),
			WithStopWhen(StepCountIs(3)),
			WithPrepareStep(func(state PrepareStepState) (*PrepareStepResult, error) {
				if state.StepNumber == 1 {
					return &PrepareStepResult{Temperature: &override}, nil
				}
				return nil, nil
			}),
		)
		for range result.FullStream() {
		}

		assert.Equal(t, []float64{0.7, 0, 0.7}, temperatures)
	})

	t.Run("provider options deep merge for the current step", func(t *testing.T) {
		var calls []provider.CallOptions
		callCount := 0
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			calls = append(calls, opts)
			callCount++
			if callCount == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("lookup", `{}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		}}
		outer := provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"type":"enabled","budgetTokens":1000},"effort":"high"}`)}
		step := provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"thinking":{"budgetTokens":2000}}`)}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("hello")),
			WithTools(ToolSet{"lookup": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				return json.RawMessage(`{"ok":true}`), nil
			}}}),
			WithProviderOptions(outer),
			WithStopWhen(StepCountIs(2)),
			WithPrepareStep(func(state PrepareStepState) (*PrepareStepResult, error) {
				if state.StepNumber == 0 {
					return &PrepareStepResult{ProviderOptions: provider.BuildProviderOptions(step)}, nil
				}
				return nil, nil
			}),
		)
		for range result.FullStream() {
		}

		require.Len(t, calls, 2)
		first, err := json.Marshal(calls[0].ProviderOptions)
		require.NoError(t, err)
		assert.JSONEq(t, `{"anthropic":{"thinking":{"type":"enabled","budgetTokens":2000},"effort":"high"}}`, string(first))
		second, err := json.Marshal(calls[1].ProviderOptions)
		require.NoError(t, err)
		assert.JSONEq(t, `{"anthropic":{"thinking":{"type":"enabled","budgetTokens":1000},"effort":"high"}}`, string(second))
	})
}

func TestStreamTextPrepareStep_ActiveToolsAndContext(t *testing.T) {
	t.Run("explicit empty active tools disables every tool", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			opts []StreamOption
		}{
			{name: "outer", opts: []StreamOption{WithActiveTools()}},
			{name: "prepare step", opts: []StreamOption{WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) {
				return &PrepareStepResult{ActiveTools: []string{}}, nil
			})}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				var call provider.CallOptions
				model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
					call = opts
					return &provider.StreamResult{Stream: textStreamParts("done")}, nil
				}}
				opts := []StreamOption{
					WithModelMessages(provider.UserText("hello")),
					WithTools(ToolSet{"lookup": {Description: "lookup"}}),
				}
				opts = append(opts, tc.opts...)
				result := StreamText(context.Background(), model, opts...)
				for range result.FullStream() {
				}
				assert.Empty(t, call.Tools)
			})
		}
	})

	t.Run("runtime context carries forward", func(t *testing.T) {
		var states []any
		var executionContext any
		callCount := 0
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			callCount++
			if callCount == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("lookup", `{}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		}}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("hello")),
			WithTools(ToolSet{"lookup": {Execute: func(_ context.Context, _ json.RawMessage, opts ToolExecutionOptions) (json.RawMessage, error) {
				executionContext = opts.Context
				return json.RawMessage(`{"ok":true}`), nil
			}}}),
			WithStopWhen(StepCountIs(2)),
			WithPrepareStep(func(state PrepareStepState) (*PrepareStepResult, error) {
				states = append(states, state.Context)
				if state.StepNumber == 0 {
					return &PrepareStepResult{Context: "step-context"}, nil
				}
				return nil, nil
			}),
		)
		for range result.FullStream() {
		}

		assert.Equal(t, []any{nil, "step-context"}, states)
		assert.Equal(t, "step-context", executionContext)
	})
}

func TestStreamTextIncompleteProviderStream(t *testing.T) {
	t.Run("first step without output", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, 2)
				ch <- provider.StreamPart{Type: provider.PartStreamStart}
				ch <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "response-1"}
				close(ch)
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		var stepFinishCount, finishCount, errorCount int
		var callbackErr error
		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("hi")),
			OnStepFinish(func(_ OnStepFinishState) { stepFinishCount++ }),
			OnFinish(func(_ OnFinishState) { finishCount++ }),
			OnError(func(err error) {
				errorCount++
				callbackErr = err
			}),
		)

		var types []string
		for part := range result.FullStream() {
			types = append(types, typeName(part))
		}

		require.ErrorIs(t, result.Err(), ErrNoOutputGenerated)
		assert.ErrorIs(t, callbackErr, ErrNoOutputGenerated)
		assert.ErrorContains(t, result.Err(), "model stream ended without a finish chunk")
		assert.Empty(t, result.Steps())
		assert.Equal(t, 0, stepFinishCount)
		assert.Equal(t, 0, finishCount)
		assert.Equal(t, 1, errorCount)
		assert.Equal(t, []string{"start", "start-step", "error"}, types)
	})

	t.Run("continuation step without output", func(t *testing.T) {
		callCount := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				callCount++
				if callCount == 1 {
					return &provider.StreamResult{Stream: toolCallStreamParts("weather", `{}`)}, nil
				}
				ch := make(chan provider.StreamPart, 2)
				ch <- provider.StreamPart{Type: provider.PartStreamStart}
				ch <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "response-2"}
				close(ch)
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		var stepFinishCount, finishCount, errorCount int
		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("weather?")),
			WithTools(ToolSet{
				"weather": Tool{Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{"temp":72}`), nil
				}},
			}),
			WithStopWhen(StepCountIs(3)),
			OnStepFinish(func(_ OnStepFinishState) { stepFinishCount++ }),
			OnFinish(func(_ OnFinishState) { finishCount++ }),
			OnError(func(error) { errorCount++ }),
		)

		var finishStepCount, streamFinishCount int
		for part := range result.FullStream() {
			switch part.(type) {
			case StreamFinishStep:
				finishStepCount++
			case StreamFinish:
				streamFinishCount++
			}
		}

		require.ErrorIs(t, result.Err(), ErrNoOutputGenerated)
		assert.Len(t, result.Steps(), 1)
		assert.Equal(t, 2, callCount)
		assert.Equal(t, 1, stepFinishCount)
		assert.Equal(t, 0, finishCount)
		assert.Equal(t, 1, errorCount)
		assert.Equal(t, 1, finishStepCount)
		assert.Equal(t, 0, streamFinishCount)
	})

	t.Run("provider error without finish emits finish step", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, 1)
				ch <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "invalid provider stream"})}
				close(ch)
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		result := StreamText(context.Background(), model, WithModelMessages(provider.UserText("hi")))
		var types []string
		for part := range result.FullStream() {
			types = append(types, typeName(part))
		}

		require.Error(t, result.Err())
		assert.Equal(t, []string{"start", "start-step", "error", "finish-step", "finish"}, types)
		assert.Equal(t, provider.FinishReasonError, result.FinishReason().Unified)
	})

	t.Run("partial output", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, 2)
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
				ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "partial"}
				close(ch)
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		var stepFinishCount, errorCount int
		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("hi")),
			OnStepFinish(func(_ OnStepFinishState) { stepFinishCount++ }),
			OnError(func(error) { errorCount++ }),
		)

		var types []string
		for part := range result.FullStream() {
			types = append(types, typeName(part))
		}

		require.NoError(t, result.Err())
		assert.Equal(t, "partial", result.Text())
		assert.Equal(t, provider.FinishReasonOther, result.FinishReason().Unified)
		assert.Len(t, result.Steps(), 1)
		assert.Equal(t, 1, stepFinishCount)
		assert.Equal(t, 0, errorCount)
		assert.Equal(t, []string{"start", "start-step", "text-start", "text-delta", "finish-step", "finish"}, types)
	})
}

func TestStreamTextMultiStepToolLoop(t *testing.T) {
	callNum := 0
	model := &mockModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			callNum++
			if callNum == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("weather", `{"city":"NYC"}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("It's 72F in NYC")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("weather?")),
		WithTools(ToolSet{
			"weather": Tool{
				Description: "Get weather",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, input json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{"temp":72}`), nil
				},
			},
		}),
		WithStopWhen(StepCountIs(5)),
	)

	for range result.FullStream() {
	}

	assert.Equal(t, "It's 72F in NYC", result.Text())
	steps := result.Steps()
	require.Len(t, steps, 2)
	assert.Equal(t, StepTypeInitial, steps[0].StepType)
	assert.Equal(t, StepTypeToolResult, steps[1].StepType)
	assert.Equal(t, 2, model.callCount)
}

func TestStreamText_ToolInputSchemaValidation(t *testing.T) {
	t.Run("invalid optional boolean empty string is not executed and can be retried", func(t *testing.T) {
		var executeCount atomic.Int32
		callNum := 0
		model := &mockModel{}
		model.streamFunc = func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			callNum++
			if callNum == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("search", `{"query_type":"range","summarize":""}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("recovered")}, nil
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("search")),
			WithTools(ToolSet{
				"search": Tool{
					Description: "Search",
					InputSchema: testMustSchema(t, `{"type":"object","additionalProperties":false,"properties":{"query_type":{"type":"string"},"summarize":{"type":"boolean"}},"required":["query_type"]}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						executeCount.Add(1)
						return json.RawMessage(`{"ok":true}`), nil
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)

		var chunks []UIMessageChunk
		for chunk := range result.ToUIMessageStream() {
			chunks = append(chunks, chunk)
		}

		require.NoError(t, result.Err())
		assert.Equal(t, int32(0), executeCount.Load())
		assert.Equal(t, "recovered", result.Text())

		steps := result.Steps()
		require.Len(t, steps, 2)
		require.Len(t, steps[0].ToolCalls, 1)
		assert.True(t, steps[0].ToolCalls[0].Invalid)
		require.Len(t, steps[0].ToolResults, 1)
		require.NotNil(t, steps[0].ToolResults[0].ModelOutput)
		assert.Contains(t, steps[0].ToolResults[0].ModelOutput.Text, "invalid input for tool search")

		var inputError, outputError *UIMessageChunk
		for i := range chunks {
			switch chunks[i].Type {
			case ChunkToolInputError:
				inputError = &chunks[i]
			case ChunkToolOutputError:
				outputError = &chunks[i]
			}
		}
		require.NotNil(t, inputError)
		require.NotNil(t, outputError)
		assert.Nil(t, inputError.Dynamic)
		assert.Nil(t, outputError.Dynamic)
		assert.Nil(t, outputError.ProviderMetadata)
	})

	t.Run("optional empty string is valid without minLength", func(t *testing.T) {
		var executeCount atomic.Int32
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: toolCallStreamParts("search", `{"query_type":"range","query":""}`)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("search")),
			WithTools(ToolSet{
				"search": Tool{
					Description: "Search",
					InputSchema: testMustSchema(t, `{"type":"object","additionalProperties":false,"properties":{"query_type":{"type":"string"},"query":{"type":"string"}},"required":["query_type"]}`),
					Execute: func(_ context.Context, input json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						executeCount.Add(1)
						assert.JSONEq(t, `{"query_type":"range","query":""}`, string(input))
						return json.RawMessage(`{"ok":true}`), nil
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)

		for range result.FullStream() {
		}

		require.NoError(t, result.Err())
		assert.Equal(t, int32(1), executeCount.Load())
		steps := result.Steps()
		require.Len(t, steps, 2)
		require.Len(t, steps[0].ToolCalls, 1)
		assert.False(t, steps[0].ToolCalls[0].Invalid)
	})

	t.Run("invalid external tool input continues to retry step", func(t *testing.T) {
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: toolCallStreamParts("external", `{"summarize":""}`)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("recovered")}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("search")),
			WithTools(ToolSet{
				"external": Tool{
					Description: "External search",
					InputSchema: testMustSchema(t, `{"type":"object","additionalProperties":false,"properties":{"summarize":{"type":"boolean"}}}`),
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)

		for range result.FullStream() {
		}

		require.NoError(t, result.Err())
		assert.Equal(t, "recovered", result.Text())
		assert.Equal(t, 2, model.callCount)
		steps := result.Steps()
		require.Len(t, steps, 2)
		require.Len(t, steps[0].ToolCalls, 1)
		assert.True(t, steps[0].ToolCalls[0].Invalid)
		require.Len(t, steps[0].ToolResults, 1)
	})

	t.Run("whitespace-only input validates as empty object", func(t *testing.T) {
		var executeCount atomic.Int32
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: toolCallStreamParts("empty", "  \n\t  ")}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("empty")),
			WithTools(ToolSet{
				"empty": Tool{
					Description: "No-arg tool",
					InputSchema: testMustSchema(t, `{"type":"object","additionalProperties":false}`),
					Execute: func(_ context.Context, input json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						executeCount.Add(1)
						assert.JSONEq(t, `{}`, string(input))
						return json.RawMessage(`{"ok":true}`), nil
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)

		for range result.FullStream() {
		}

		require.NoError(t, result.Err())
		assert.Equal(t, int32(1), executeCount.Load())
	})

	t.Run("invalid provider executed tool call preserves provider result without synthetic client result", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, 4)
				go func() {
					defer close(ch)
					ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "server", Input: `{"summarize":""}`, ProviderExecuted: true}
					ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "server", Result: json.RawMessage(`{"type":"server_tool_result_error","errorCode":"invalid_tool_input"}`), IsError: true, ProviderExecuted: true}
					ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
				}()
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("server")),
			WithTools(ToolSet{
				"server": Tool{
					Description: "Server tool",
					InputSchema: testMustSchema(t, `{"type":"object","additionalProperties":false,"properties":{"summarize":{"type":"boolean"}}}`),
				},
			}),
			WithStopWhen(StepCountIs(1)),
		)

		var streamCalls []StreamToolCall
		var chunks []UIMessageChunk
		for part := range result.FullStream() {
			if call, ok := part.(StreamToolCall); ok {
				streamCalls = append(streamCalls, call)
			}
			chunks = append(chunks, translateToChunks(part, uiMessageStreamConfig{})...)
		}

		require.NoError(t, result.Err())
		require.Len(t, streamCalls, 1)
		assert.True(t, streamCalls[0].Invalid)
		assert.Error(t, streamCalls[0].Error)
		assert.Equal(t, boolPtr(true), streamCalls[0].Dynamic)
		var inputErrorCount, outputErrorCount int
		for _, chunk := range chunks {
			switch chunk.Type {
			case ChunkToolInputError:
				inputErrorCount++
			case ChunkToolOutputError:
				outputErrorCount++
			}
		}
		assert.Equal(t, 1, inputErrorCount)
		assert.Equal(t, 1, outputErrorCount)
		step := result.Steps()[0]
		require.Len(t, step.ToolCalls, 1)
		assert.True(t, step.ToolCalls[0].Invalid)
		assert.Error(t, step.ToolCalls[0].Error)
		assert.True(t, step.ToolCalls[0].ProviderExecuted)
		assert.Equal(t, boolPtr(true), step.ToolCalls[0].Dynamic)
		require.Len(t, step.ToolResults, 1)
		assert.True(t, step.ToolResults[0].ProviderExecuted)
		assert.True(t, step.ToolResults[0].IsError)
		assert.Error(t, step.ToolResults[0].Error)
		require.Len(t, step.Content, 2)
		callContent, ok := step.Content[0].(ToolCallContent)
		require.True(t, ok)
		assert.True(t, callContent.Invalid)
		assert.Error(t, callContent.Error)
		errorContent, ok := step.Content[1].(ToolErrorContent)
		require.True(t, ok)
		rawError, ok := errorContent.Error.(json.RawMessage)
		require.True(t, ok)
		assert.JSONEq(t, `{"type":"server_tool_result_error","errorCode":"invalid_tool_input"}`, string(rawError))
		assert.True(t, errorContent.ProviderExecuted)

		messages := step.Response.Messages
		require.Len(t, messages, 1)
		assert.Equal(t, provider.RoleAssistant, messages[0].Role)
		require.Len(t, messages[0].Content, 2)
		assert.Equal(t, provider.ContentPartTypeToolCall, messages[0].Content[0].Type)
		assert.True(t, messages[0].Content[0].ProviderExecuted)
		assert.Equal(t, provider.ContentPartTypeToolResult, messages[0].Content[1].Type)
	})

	t.Run("GenerateText preserves invalid provider call and provider result", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 4)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "server", Input: `{"summarize":""}`, ProviderExecuted: true}
				ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "server", Result: json.RawMessage(`{"type":"server_tool_result_error","errorCode":"invalid_tool_input"}`), IsError: true, ProviderExecuted: true}
				ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		}}

		result, err := GenerateText(context.Background(), model,
			WithModelMessages(provider.UserText("server")),
			WithTools(ToolSet{"server": Tool{InputSchema: testMustSchema(t, `{"type":"object","properties":{"summarize":{"type":"boolean"}}}`)}}),
		)
		require.NoError(t, err)
		require.Len(t, result.ToolCalls, 1)
		assert.True(t, result.ToolCalls[0].Invalid)
		assert.Error(t, result.ToolCalls[0].Error)
		assert.True(t, result.ToolCalls[0].ProviderExecuted)
		assert.Equal(t, boolPtr(true), result.ToolCalls[0].Dynamic)
		require.Len(t, result.ToolResults, 1)
		assert.True(t, result.ToolResults[0].ProviderExecuted)
		require.Len(t, result.Content, 2)
		callContent, ok := result.Content[0].(ToolCallContent)
		require.True(t, ok)
		assert.True(t, callContent.Invalid)
		assert.Error(t, callContent.Error)
		errorContent, ok := result.Content[1].(ToolErrorContent)
		require.True(t, ok)
		rawError, ok := errorContent.Error.(json.RawMessage)
		require.True(t, ok)
		assert.JSONEq(t, `{"type":"server_tool_result_error","errorCode":"invalid_tool_input"}`, string(rawError))
		assert.True(t, errorContent.ProviderExecuted)
	})

	t.Run("malformed provider executed input does not reuse the previous call", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 5)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "server", Input: `{"summarize":true}`, ProviderExecuted: true}
				ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c2", ToolName: "server", Input: `{`, ProviderExecuted: true}
				ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "c2", ToolName: "server", Result: json.RawMessage(`{"type":"server_tool_result_error","errorCode":"invalid_tool_input"}`), IsError: true, ProviderExecuted: true}
				ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		}}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("server")),
			WithTools(ToolSet{"server": Tool{InputSchema: testMustSchema(t, `{"type":"object","properties":{"summarize":{"type":"boolean"}}}`)}}),
		)
		for range result.FullStream() {
		}

		require.NoError(t, result.Err())
		step := result.Steps()[0]
		require.Len(t, step.ToolCalls, 2)
		assert.Equal(t, "c1", step.ToolCalls[0].ToolCallID)
		assert.False(t, step.ToolCalls[0].Invalid)
		assert.Equal(t, "c2", step.ToolCalls[1].ToolCallID)
		assert.True(t, step.ToolCalls[1].Invalid)
		assert.Error(t, step.ToolCalls[1].Error)
		require.Len(t, step.Response.Messages, 1)
		require.Len(t, step.Response.Messages[0].Content, 3)
		assert.Equal(t, "c1", step.Response.Messages[0].Content[0].ToolCallID)
		assert.Equal(t, "c2", step.Response.Messages[0].Content[1].ToolCallID)
		assert.Equal(t, "c2", step.Response.Messages[0].Content[2].ToolCallID)
	})

	t.Run("GenerateText preserves malformed provider call and provider result", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 4)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "server", Input: `{`, ProviderExecuted: true}
				ch <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "c1", ToolName: "server", Result: json.RawMessage(`{"type":"server_tool_result_error","errorCode":"invalid_tool_input"}`), IsError: true, ProviderExecuted: true}
				ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		}}

		result, err := GenerateText(context.Background(), model,
			WithModelMessages(provider.UserText("server")),
			WithTools(ToolSet{"server": Tool{InputSchema: testMustSchema(t, `{"type":"object"}`)}}),
		)
		require.NoError(t, err)
		require.Len(t, result.ToolCalls, 1)
		assert.True(t, result.ToolCalls[0].Invalid)
		assert.Error(t, result.ToolCalls[0].Error)
		require.Len(t, result.ToolResults, 1)
		assert.True(t, result.ToolResults[0].ProviderExecuted)
	})
}

func TestStreamTextMultiStepPreservesReasoningSignatureDelta(t *testing.T) {
	callNum := 0
	var secondPrompt []provider.Message
	sigMeta := provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"signature":"sig_xyz"}`),
	}
	model := &mockModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			callNum++
			if callNum == 2 {
				secondPrompt = opts.Prompt
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}

			ch := make(chan provider.StreamPart, 8)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartReasoningStart, ID: "r1"}
				ch <- provider.StreamPart{Type: provider.PartReasoningDelta, ID: "r1", Delta: "thinking step"}
				ch <- provider.StreamPart{Type: provider.PartReasoningDelta, ID: "r1", ProviderMetadata: sigMeta}
				ch <- provider.StreamPart{Type: provider.PartReasoningEnd, ID: "r1"}
				ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "weather", Input: `{"city":"SF"}`}
				ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("weather?")),
		WithTools(ToolSet{
			"weather": Tool{
				Description: "Get weather",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{"temp":57}`), nil
				},
			},
		}),
		WithStopWhen(StepCountIs(2)),
	)

	for range result.FullStream() {
	}

	require.NoError(t, result.Err())
	require.Len(t, result.Steps(), 2)
	require.Len(t, secondPrompt, 3)
	assistant := secondPrompt[1]
	require.Equal(t, provider.RoleAssistant, assistant.Role)
	require.Len(t, assistant.Content, 2)
	reasoning := assistant.Content[0]
	require.Equal(t, provider.ContentPartTypeReasoning, reasoning.Type)
	assert.Equal(t, "thinking step", reasoning.Text)
	raw, ok := reasoning.ProviderOptions["anthropic"].(provider.RawProviderOption)
	require.True(t, ok)
	assert.JSONEq(t, `{"signature":"sig_xyz"}`, string(raw.Raw))
}

func TestStreamTextExternalToolStopsLoop(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("external", `{"q":"test"}`)}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("search")),
		WithTools(ToolSet{
			"external": Tool{
				Description: "External tool",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
			},
		}),
		WithStopWhen(StepCountIs(5)),
	)

	for range result.FullStream() {
	}

	steps := result.Steps()
	require.Len(t, steps, 1, "loop should stop for external tool")
	assert.Len(t, steps[0].ToolCalls, 1)
}

func TestStreamTextToolApproval_RequestStopsLoop(t *testing.T) {
	var executeCount atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("dangerous", `{"amount":100}`)}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("transfer money")),
		WithGenerateID(func() string { return "apr_1" }),
		WithTools(ToolSet{
			"dangerous": Tool{
				Description:   "Transfer money",
				InputSchema:   testMustSchema(t, `{"type":"object"}`),
				NeedsApproval: ApprovalRequired(),
				Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					executeCount.Add(1)
					return json.RawMessage(`{"ok":true}`), nil
				},
			},
		}),
		WithStopWhen(StepCountIs(5)),
	)

	var approval StreamToolApprovalRequest
	for part := range result.FullStream() {
		if p, ok := part.(StreamToolApprovalRequest); ok {
			approval = p
		}
	}

	assert.Equal(t, int32(0), executeCount.Load())
	assert.Equal(t, "apr_1", approval.ApprovalID)
	assert.Equal(t, "c1", approval.ToolCallID)
	assert.Equal(t, 1, model.callCount)
	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].ToolApprovalRequests, 1)
	assert.Len(t, steps[0].ToolResults, 0)
	content := result.Content()
	require.Len(t, content, 2)
	_, ok := content[1].(ToolApprovalRequestContent)
	assert.True(t, ok)

	responseMessages := steps[0].Response.Messages
	require.Len(t, responseMessages, 1)
	assert.Equal(t, provider.RoleAssistant, responseMessages[0].Role)
	require.Len(t, responseMessages[0].Content, 2)
	assert.Equal(t, provider.ContentPartTypeToolCall, responseMessages[0].Content[0].Type)
	assert.Equal(t, provider.ContentPartTypeToolApprovalRequest, responseMessages[0].Content[1].Type)
	assert.Equal(t, "apr_1", responseMessages[0].Content[1].ApprovalID)
	assert.Equal(t, "c1", responseMessages[0].Content[1].ToolCallID)
}

func TestStreamTextToolApproval_SecretSignsRequests(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("dangerous", `{"amount":100}`)}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("transfer money")),
		WithGenerateID(func() string { return "apr_1" }),
		WithToolApprovalSecret("secret"),
		WithTools(ToolSet{
			"dangerous": Tool{
				InputSchema:   testMustSchema(t, `{"type":"object"}`),
				NeedsApproval: ApprovalRequired(),
			},
		}),
	)

	var approval StreamToolApprovalRequest
	for part := range result.FullStream() {
		if p, ok := part.(StreamToolApprovalRequest); ok {
			approval = p
		}
	}
	require.NoError(t, result.Err())
	require.NotEmpty(t, approval.Signature)
	valid, err := verifyToolApprovalSignature([]byte("secret"), approval.Signature, approval.ApprovalID, approval.ToolCallID, approval.ToolName, approval.Input)
	require.NoError(t, err)
	assert.True(t, valid)

	steps := result.Steps()
	require.Len(t, steps, 1)
	responseMessages := steps[0].Response.Messages
	require.Len(t, responseMessages, 1)
	require.Len(t, responseMessages[0].Content, 2)
	assert.Equal(t, approval.Signature, responseMessages[0].Content[1].Signature)
}

func TestStreamTextToolApproval_SecretVerifiesReplay(t *testing.T) {
	signature, err := signToolApproval([]byte("secret"), "apr_1", "c1", "dangerous", json.RawMessage(`{"amount":100}`))
	require.NoError(t, err)

	tests := []struct {
		name      string
		signature string
		wantErr   string
	}{
		{name: "valid", signature: signature},
		{name: "missing", wantErr: "missing signature"},
		{name: "invalid", signature: "not-valid", wantErr: "invalid signature"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			approved := true
			var executed atomic.Int32
			model := &mockModel{
				streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: finishStreamParts()}, nil
				},
			}

			result := StreamText(context.Background(), model,
				WithModelMessages(
					provider.NewAssistantMessage(
						provider.ToolCallPart("c1", "dangerous", json.RawMessage(`{"amount":100}`)),
						provider.ContentPart{Type: provider.ContentPartTypeToolApprovalRequest, ApprovalID: "apr_1", ToolCallID: "c1", ToolName: "dangerous", Signature: tc.signature},
					),
					provider.NewToolMessage(provider.ToolApprovalResponsePart("apr_1", approved, "ok")),
				),
				WithToolApprovalSecret("secret"),
				WithTools(ToolSet{"dangerous": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					executed.Add(1)
					return json.RawMessage(`{"ok":true}`), nil
				}}}),
			)
			for range result.FullStream() {
			}

			if tc.wantErr == "" {
				require.NoError(t, result.Err())
				assert.Equal(t, int32(1), executed.Load())
				return
			}
			require.Error(t, result.Err())
			assert.Contains(t, result.Err().Error(), tc.wantErr)
			assert.Equal(t, int32(0), executed.Load())
		})
	}
}

func TestStreamTextToolApproval_ApprovedReplayRevalidatesInput(t *testing.T) {
	input := json.RawMessage(`{"amount":"bad"}`)
	signature, err := signToolApproval([]byte("secret"), "apr_1", "c1", "dangerous", input)
	require.NoError(t, err)
	approved := true
	var executed atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 1)
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(
			provider.NewAssistantMessage(
				provider.ToolCallPart("c1", "dangerous", input),
				provider.ContentPart{Type: provider.ContentPartTypeToolApprovalRequest, ApprovalID: "apr_1", ToolCallID: "c1", ToolName: "dangerous", Signature: signature},
			),
			provider.NewToolMessage(provider.ToolApprovalResponsePart("apr_1", approved, "ok")),
		),
		WithToolApprovalSecret("secret"),
		WithTools(ToolSet{"dangerous": Tool{
			InputSchema: testMustSchema(t, `{"type":"object","properties":{"amount":{"type":"number"}},"required":["amount"]}`),
			Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				executed.Add(1)
				return json.RawMessage(`{"ok":true}`), nil
			},
		}}),
	)
	for range result.FullStream() {
	}

	require.Error(t, result.Err())
	assert.Contains(t, result.Err().Error(), "invalid input for tool dangerous")
	assert.Equal(t, int32(0), executed.Load())
}

func TestStreamTextToolApproval_ApprovedReplayRunsCustomValidationForNonObjectInput(t *testing.T) {
	input := json.RawMessage(`"bad"`)
	approved := true
	var executed atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 1)
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(
			provider.NewAssistantMessage(
				provider.ToolCallPart("c1", "dangerous", input),
				provider.ContentPart{Type: provider.ContentPartTypeToolApprovalRequest, ApprovalID: "apr_1", ToolCallID: "c1", ToolName: "dangerous"},
			),
			provider.NewToolMessage(provider.ToolApprovalResponsePart("apr_1", approved, "ok")),
		),
		WithTools(ToolSet{"dangerous": Tool{
			InputSchema: testMustSchema(t, `{"type":"object"}`),
			ValidateInput: func(input json.RawMessage) error {
				if !isJSONObject(input) {
					return fmt.Errorf("input must be an object")
				}
				return nil
			},
			Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				executed.Add(1)
				return json.RawMessage(`{"ok":true}`), nil
			},
		}}),
	)
	for range result.FullStream() {
	}

	require.Error(t, result.Err())
	assert.Contains(t, result.Err().Error(), "input must be an object")
	assert.Equal(t, int32(0), executed.Load())
}

func TestStreamTextToolApproval_ApprovedReplayRechecksPolicy(t *testing.T) {
	input := json.RawMessage(`{"amount":100}`)
	signature, err := signToolApproval([]byte("secret"), "apr_1", "c1", "dangerous", input)
	require.NoError(t, err)
	approved := true
	var executed atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			last := opts.Prompt[len(opts.Prompt)-1]
			require.Equal(t, provider.RoleTool, last.Role)
			require.Len(t, last.Content, 1)
			assert.Equal(t, provider.ContentPartTypeToolResult, last.Content[0].Type)
			require.NotNil(t, last.Content[0].Output)
			assert.Equal(t, provider.ToolOutputExecutionDenied, last.Content[0].Output.Type)
			assert.Equal(t, "policy changed", last.Content[0].Output.Reason)
			return &provider.StreamResult{Stream: finishStreamParts()}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(
			provider.NewAssistantMessage(
				provider.ToolCallPart("c1", "dangerous", input),
				provider.ContentPart{Type: provider.ContentPartTypeToolApprovalRequest, ApprovalID: "apr_1", ToolCallID: "c1", ToolName: "dangerous", Signature: signature},
			),
			provider.NewToolMessage(provider.ToolApprovalResponsePart("apr_1", approved, "ok")),
		),
		WithToolApprovalSecret("secret"),
		WithToolApproval(ToolApprovalMap{"dangerous": ApprovalPolicy(ToolApprovalDenied, "policy changed")}),
		WithTools(ToolSet{"dangerous": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executed.Add(1)
			return json.RawMessage(`{"ok":true}`), nil
		}}}),
	)
	for range result.FullStream() {
	}

	require.NoError(t, result.Err())
	assert.Equal(t, int32(0), executed.Load())
}

func TestStreamTextToolApproval_SecretDoesNotVerifyDeniedReplay(t *testing.T) {
	approved := false
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: finishStreamParts()}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(
			provider.NewAssistantMessage(
				provider.ToolCallPart("c1", "dangerous", json.RawMessage(`{"amount":100}`)),
				provider.ContentPart{Type: provider.ContentPartTypeToolApprovalRequest, ApprovalID: "apr_1", ToolCallID: "c1", ToolName: "dangerous"},
			),
			provider.NewToolMessage(provider.ToolApprovalResponsePart("apr_1", approved, "blocked")),
		),
		WithToolApprovalSecret("secret"),
		WithTools(ToolSet{"dangerous": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		}}}),
	)

	var denied StreamToolOutputDenied
	for part := range result.FullStream() {
		if p, ok := part.(StreamToolOutputDenied); ok {
			denied = p
		}
	}

	require.NoError(t, result.Err())
	assert.Equal(t, "c1", denied.ToolCallID)
	assert.Equal(t, "dangerous", denied.ToolName)
}

func TestStreamTextToolApproval_RequestForNonExecutableTools(t *testing.T) {
	t.Run("external client tool", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: toolCallStreamParts("external", `{}`)}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("search")),
			WithGenerateID(func() string { return "apr_1" }),
			WithTools(ToolSet{"external": Tool{NeedsApproval: ApprovalRequired()}}),
		)
		var approval StreamToolApprovalRequest
		for part := range result.FullStream() {
			if p, ok := part.(StreamToolApprovalRequest); ok {
				approval = p
			}
		}

		assert.Equal(t, "apr_1", approval.ApprovalID)
		assert.Equal(t, "c1", approval.ToolCallID)
		assert.False(t, approval.ProviderExecuted)
		steps := result.Steps()
		require.Len(t, steps, 1)
		require.Len(t, steps[0].ToolApprovalRequests, 1)
		assert.Len(t, steps[0].ToolResults, 0)
	})

	t.Run("provider executed tool", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, 2)
				go func() {
					defer close(ch)
					ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "server", Input: `{}`, ProviderExecuted: true}
					ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
				}()
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("run server tool")),
			WithGenerateID(func() string { return "apr_1" }),
			WithTools(ToolSet{"server": Tool{Type: UserToolProvider, NeedsApproval: ApprovalRequired()}}),
		)
		var approval StreamToolApprovalRequest
		for part := range result.FullStream() {
			if p, ok := part.(StreamToolApprovalRequest); ok {
				approval = p
			}
		}

		assert.Equal(t, "apr_1", approval.ApprovalID)
		assert.Equal(t, "c1", approval.ToolCallID)
		assert.True(t, approval.ProviderExecuted)
	})
}

func TestStreamTextToolApproval_MixedBlockedAndUnblockedTools(t *testing.T) {
	var safeCount atomic.Int32
	var dangerousCount atomic.Int32
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 4)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "dangerous", Input: `{}`}
				ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c2", ToolName: "safe", Input: `{}`}
				ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("do things")),
		WithGenerateID(func() string { return "apr_1" }),
		WithTools(ToolSet{
			"dangerous": Tool{NeedsApproval: ApprovalRequired(), Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				dangerousCount.Add(1)
				return json.RawMessage(`{}`), nil
			}},
			"safe": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				safeCount.Add(1)
				return json.RawMessage(`{"safe":true}`), nil
			}},
		}),
		WithStopWhen(StepCountIs(5)),
	)
	// Lock Go's per-step emission order: tool-call A, tool-call B, then approval
	// request A, then tool-result B. This intentionally differs from upstream's
	// per-chunk adjacency (where each tool-call is followed immediately by its
	// approval request/response) because Go batches approval handling after the
	// provider's PartFinish to keep tool execution out of the stream-read loop.
	var emissions []string
	for part := range result.FullStream() {
		switch p := part.(type) {
		case StreamToolCall:
			emissions = append(emissions, "tool-call:"+p.ToolCallID)
		case StreamToolApprovalRequest:
			emissions = append(emissions, "approval-request:"+p.ToolCallID)
		case StreamToolResult:
			emissions = append(emissions, "tool-result:"+p.ToolCallID)
		}
	}

	assert.Equal(t, int32(0), dangerousCount.Load())
	assert.Equal(t, int32(1), safeCount.Load())
	assert.Equal(t, 1, model.callCount)
	steps := result.Steps()
	require.Len(t, steps, 1)
	assert.Len(t, steps[0].ToolApprovalRequests, 1)
	assert.Len(t, steps[0].ToolResults, 1)
	assert.Equal(t, []string{
		"tool-call:c1",
		"tool-call:c2",
		"approval-request:c1",
		"tool-result:c2",
	}, emissions)
}

func TestStreamTextToolApproval_DynamicError(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("dangerous", `{}`)}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("do thing")),
		WithTools(ToolSet{
			"dangerous": Tool{
				NeedsApproval: ApprovalIf(func(json.RawMessage, ToolExecutionOptions) (bool, error) {
					return false, fmt.Errorf("approval failed")
				}),
				Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{}`), nil
				},
			},
		}),
	)
	for range result.FullStream() {
	}

	require.Error(t, result.Err())
	assert.Contains(t, result.Err().Error(), "approval failed")
}

func TestStreamTextToolApproval_ProviderRequestSurfaced(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 6)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "server_tool", Input: `{}`, ProviderExecuted: true}
				ch <- provider.StreamPart{Type: provider.PartToolApprovalRequest, ApprovalID: "apr_1", ToolCallID: "c1"}
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
				ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "after approval"}
				ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
				ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model, WithModelMessages(provider.UserText("run server tool")))
	var approval StreamToolApprovalRequest
	for part := range result.FullStream() {
		if p, ok := part.(StreamToolApprovalRequest); ok {
			approval = p
		}
	}

	assert.Equal(t, "apr_1", approval.ApprovalID)
	assert.Equal(t, "c1", approval.ToolCallID)
	assert.True(t, approval.ProviderExecuted)
	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].ToolApprovalRequests, 1)
	require.Len(t, steps[0].Content, 3)
	assert.IsType(t, ToolCallContent{}, steps[0].Content[0])
	assert.IsType(t, ToolApprovalRequestContent{}, steps[0].Content[1])
	assert.IsType(t, TextContent{}, steps[0].Content[2])

	require.Len(t, steps[0].Response.Messages, 1)
	responseContent := steps[0].Response.Messages[0].Content
	require.Len(t, responseContent, 3)
	assert.Equal(t, provider.ContentPartTypeToolCall, responseContent[0].Type)
	assert.Equal(t, provider.ContentPartTypeToolApprovalRequest, responseContent[1].Type)
	assert.Equal(t, "apr_1", responseContent[1].ApprovalID)
	assert.Equal(t, provider.ContentPartTypeText, responseContent[2].Type)
}

func TestStreamTextToolCall_UsesInputStartTitleFallback(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 4)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: "c1", ToolName: "search", Title: "Search docs"}
				ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "search", Input: `{}`}
				ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("search")),
		WithTools(ToolSet{"search": Tool{}}),
	)
	var toolCall StreamToolCall
	for part := range result.FullStream() {
		if p, ok := part.(StreamToolCall); ok {
			toolCall = p
		}
	}

	assert.Equal(t, "Search docs", toolCall.Title)
}

func TestStreamTextToolApproval_ApprovedResponseExecutesBeforeModelCall(t *testing.T) {
	approved := true
	msgs := []provider.Message{
		provider.UserText("please transfer"),
		provider.NewAssistantMessage(
			provider.ToolCallPart("c1", "dangerous", json.RawMessage(`{"amount":100}`)),
			provider.ToolApprovalRequestPart("apr_1", "c1", false),
		),
		provider.NewToolMessage(provider.ToolApprovalResponsePart("apr_1", approved, "ok")),
	}
	var capturedPrompt []provider.Message
	var prepareState PrepareStepState
	var executeCount atomic.Int32
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		capturedPrompt = opts.Prompt
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(msgs...),
		WithTools(ToolSet{"dangerous": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executeCount.Add(1)
			return json.RawMessage(`{"ok":true}`), nil
		}}}),
		WithPrepareStep(func(state PrepareStepState) (*PrepareStepResult, error) {
			prepareState = state
			return nil, nil
		}),
	)
	var sawResult bool
	for part := range result.FullStream() {
		if _, ok := part.(StreamToolResult); ok {
			sawResult = true
		}
	}

	assert.True(t, sawResult)
	assert.Equal(t, int32(1), executeCount.Load())
	assert.Equal(t, msgs, prepareState.InitialMessages)
	require.Len(t, prepareState.ResponseMessages, 1)
	assert.Equal(t, provider.RoleTool, prepareState.ResponseMessages[0].Role)
	assert.Equal(t, append(cloneMessages(msgs), prepareState.ResponseMessages...), prepareState.Messages)
	require.NotEmpty(t, capturedPrompt)
	last := capturedPrompt[len(capturedPrompt)-1]
	require.Equal(t, provider.RoleTool, last.Role)
	require.Len(t, last.Content, 1)
	assert.Equal(t, provider.ContentPartTypeToolResult, last.Content[0].Type)
	assert.Equal(t, "c1", last.Content[0].ToolCallID)
}

func TestStreamTextToolApproval_DeniedResponseCreatesExecutionDenied(t *testing.T) {
	approved := false
	msgs := []provider.Message{
		provider.UserText("please transfer"),
		provider.NewAssistantMessage(
			provider.ToolCallPart("c1", "dangerous", json.RawMessage(`{}`)),
			provider.ToolApprovalRequestPart("apr_1", "c1", false),
		),
		provider.NewToolMessage(provider.ToolApprovalResponsePart("apr_1", approved, "too risky")),
	}
	var capturedPrompt []provider.Message
	var executeCount atomic.Int32
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		capturedPrompt = opts.Prompt
		return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(msgs...),
		WithTools(ToolSet{"dangerous": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executeCount.Add(1)
			return json.RawMessage(`{}`), nil
		}}}),
	)
	for range result.FullStream() {
	}

	assert.Equal(t, int32(0), executeCount.Load())
	require.NotEmpty(t, capturedPrompt)
	last := capturedPrompt[len(capturedPrompt)-1]
	require.Equal(t, provider.RoleTool, last.Role)
	require.Len(t, last.Content, 1)
	require.NotNil(t, last.Content[0].Output)
	assert.Equal(t, provider.ToolOutputExecutionDenied, last.Content[0].Output.Type)
	assert.Equal(t, "too risky", last.Content[0].Output.Reason)
}

func TestStreamTextToolApproval_DeniedResponseWithExistingResultEmitsOutputDenied(t *testing.T) {
	approved := false
	msgs := []provider.Message{
		provider.UserText("please transfer"),
		provider.NewAssistantMessage(
			provider.ToolCallPart("c1", "dangerous", json.RawMessage(`{}`)),
			provider.ToolApprovalRequestPart("apr_1", "c1", false),
		),
		provider.NewToolMessage(
			provider.ToolApprovalResponsePart("apr_1", approved, "too risky"),
			provider.ToolResultPart("c1", "dangerous", &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: "too risky"}),
		),
	}
	var capturedPrompt []provider.Message
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		capturedPrompt = opts.Prompt
		return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
	}}

	result := StreamText(context.Background(), model, WithModelMessages(msgs...))
	var deniedEvents []StreamToolOutputDenied
	for part := range result.FullStream() {
		if denied, ok := part.(StreamToolOutputDenied); ok {
			deniedEvents = append(deniedEvents, denied)
		}
	}

	require.Len(t, deniedEvents, 1)
	assert.Equal(t, "c1", deniedEvents[0].ToolCallID)
	require.NotEmpty(t, capturedPrompt)
	last := capturedPrompt[len(capturedPrompt)-1]
	require.Equal(t, provider.RoleTool, last.Role)
	require.Len(t, last.Content, 1)
	require.NotNil(t, last.Content[0].Output)
	assert.Equal(t, provider.ToolOutputExecutionDenied, last.Content[0].Output.Type)
}

func TestStreamTextToolApproval_DeniedResponseWithUnrelatedResultIsNotReprocessed(t *testing.T) {
	approved := false
	msgs := []provider.Message{
		provider.UserText("please transfer"),
		provider.NewAssistantMessage(
			provider.ToolCallPart("c1", "dangerous", json.RawMessage(`{}`)),
			provider.ToolApprovalRequestPart("apr_1", "c1", false),
		),
		provider.NewToolMessage(
			provider.ToolApprovalResponsePart("apr_1", approved, "too risky"),
			provider.ToolResultPart("c1", "dangerous", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"already":true}`)}),
		),
	}
	var capturedPrompt []provider.Message
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		capturedPrompt = opts.Prompt
		return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
	}}

	result := StreamText(context.Background(), model, WithModelMessages(msgs...))
	var deniedEvents []StreamToolOutputDenied
	for part := range result.FullStream() {
		if denied, ok := part.(StreamToolOutputDenied); ok {
			deniedEvents = append(deniedEvents, denied)
		}
	}

	assert.Empty(t, deniedEvents)
	require.Len(t, capturedPrompt, len(msgs))
	last := capturedPrompt[len(capturedPrompt)-1]
	require.Equal(t, provider.RoleTool, last.Role)
	require.Len(t, last.Content, 1)
	assert.Equal(t, provider.ToolOutputJSON, last.Content[0].Output.Type)
}

func TestStreamTextToolApproval_MixedResumeEmitsDeniedBeforeApproved(t *testing.T) {
	approved := true
	denied := false
	msgs := []provider.Message{
		provider.UserText("please handle both"),
		provider.NewAssistantMessage(
			provider.ToolCallPart("c-approved", "safe", json.RawMessage(`{}`)),
			provider.ToolApprovalRequestPart("apr_approved", "c-approved", false),
			provider.ToolCallPart("c-denied", "dangerous", json.RawMessage(`{}`)),
			provider.ToolApprovalRequestPart("apr_denied", "c-denied", false),
		),
		provider.NewToolMessage(
			provider.ToolApprovalResponsePart("apr_approved", approved, "ok"),
			provider.ToolApprovalResponsePart("apr_denied", denied, "too risky"),
		),
	}
	var capturedPrompt []provider.Message
	var safeCount atomic.Int32
	var dangerousCount atomic.Int32
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		capturedPrompt = opts.Prompt
		return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(msgs...),
		WithTools(ToolSet{
			"safe": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				safeCount.Add(1)
				return json.RawMessage(`{"ok":true}`), nil
			}},
			"dangerous": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				dangerousCount.Add(1)
				return json.RawMessage(`{}`), nil
			}},
		}),
	)
	var eventOrder []string
	for part := range result.FullStream() {
		switch p := part.(type) {
		case StreamToolOutputDenied:
			eventOrder = append(eventOrder, "denied:"+p.ToolCallID)
		case StreamToolResult:
			eventOrder = append(eventOrder, "result:"+p.ToolCallID)
		}
	}

	assert.Equal(t, []string{"denied:c-denied", "result:c-approved"}, eventOrder)
	assert.Equal(t, int32(1), safeCount.Load())
	assert.Equal(t, int32(0), dangerousCount.Load())
	require.NotEmpty(t, capturedPrompt)
	last := capturedPrompt[len(capturedPrompt)-1]
	require.Equal(t, provider.RoleTool, last.Role)
	require.Len(t, last.Content, 2)
	assert.Equal(t, provider.ContentPartTypeToolResult, last.Content[0].Type)
	assert.Equal(t, "c-approved", last.Content[0].ToolCallID)
	assert.Equal(t, provider.ContentPartTypeToolResult, last.Content[1].Type)
	assert.Equal(t, "c-denied", last.Content[1].ToolCallID)
	require.NotNil(t, last.Content[1].Output)
	assert.Equal(t, provider.ToolOutputExecutionDenied, last.Content[1].Output.Type)
}

func TestStreamTextToolApproval_ResumeApprovedToolsRunInParallel(t *testing.T) {
	approved := true
	msgs := []provider.Message{
		provider.UserText("run two slow tools"),
		provider.NewAssistantMessage(
			provider.ToolCallPart("c1", "slow", json.RawMessage(`{}`)),
			provider.ToolApprovalRequestPart("apr_1", "c1", false),
			provider.ToolCallPart("c2", "slow", json.RawMessage(`{}`)),
			provider.ToolApprovalRequestPart("apr_2", "c2", false),
		),
		provider.NewToolMessage(
			provider.ToolApprovalResponsePart("apr_1", approved, "ok"),
			provider.ToolApprovalResponsePart("apr_2", approved, "ok"),
		),
	}

	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}

	const sleepDur = 100 * time.Millisecond
	var inFlight atomic.Int32
	var maxConcurrent atomic.Int32
	execute := func(ctx context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
		current := inFlight.Add(1)
		for {
			prev := maxConcurrent.Load()
			if current <= prev || maxConcurrent.CompareAndSwap(prev, current) {
				break
			}
		}
		time.Sleep(sleepDur)
		inFlight.Add(-1)
		return json.RawMessage(`{"ok":true}`), nil
	}

	start := time.Now()
	result := StreamText(context.Background(), model,
		WithModelMessages(msgs...),
		WithTools(ToolSet{"slow": Tool{Execute: execute}}),
	)
	for range result.FullStream() {
	}
	elapsed := time.Since(start)

	assert.Equal(t, int32(2), maxConcurrent.Load(), "resume should run approved tools concurrently")
	assert.Less(t, elapsed, sleepDur*2, "resume execution should overlap; got %s", elapsed)
}

func TestStreamTextToolApproval_ResponseChunkCarriesProviderMetadata(t *testing.T) {
	meta := provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"caller":{"type":"direct"}}`),
	}
	part := StreamToolApprovalResponse{ApprovalID: "apr_1", Approved: false, Reason: "blocked", ProviderMetadata: meta}
	chunks := translateToChunks(part, uiMessageStreamConfig{})
	require.Len(t, chunks, 1)
	assert.Equal(t, ChunkToolApprovalResponse, chunks[0].Type)
	assert.Equal(t, meta, chunks[0].ProviderMetadata)

	data, err := json.Marshal(chunks[0])
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "tool-approval-response", decoded["type"])
	assert.Equal(t, false, decoded["approved"], "denial response must always serialize approved")
	pm, ok := decoded["providerMetadata"].(map[string]any)
	require.True(t, ok, "providerMetadata should be present in chunk JSON")
	assert.Contains(t, pm, "anthropic")
}

func TestStreamTextToolApproval_ProviderExecutedDeniedResponseEmitsDenied(t *testing.T) {
	approved := false
	msgs := []provider.Message{
		provider.UserText("please run server tool"),
		provider.NewAssistantMessage(
			provider.ContentPart{Type: provider.ContentPartTypeToolCall, ToolCallID: "c1", ToolName: "server", Input: json.RawMessage(`{}`), ProviderExecuted: true},
			provider.ToolApprovalRequestPart("apr_1", "c1", false),
		),
		provider.NewToolMessage(provider.ProviderExecutedToolApprovalResponsePart("apr_1", approved, "not allowed")),
	}
	var capturedPrompt []provider.Message
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		capturedPrompt = opts.Prompt
		return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(msgs...),
		WithTools(ToolSet{"server": Tool{Type: UserToolProvider}}),
	)
	var sawDenied bool
	for part := range result.FullStream() {
		if _, ok := part.(StreamToolOutputDenied); ok {
			sawDenied = true
		}
	}

	assert.True(t, sawDenied)
	require.NotEmpty(t, capturedPrompt)
	last := capturedPrompt[len(capturedPrompt)-1]
	require.Equal(t, provider.RoleTool, last.Role)
	require.Len(t, last.Content, 1)
	assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, last.Content[0].Type)
	assert.True(t, last.Content[0].ProviderExecuted)
}

func TestStreamTextToolApproval_ExistingResultNotDuplicated(t *testing.T) {
	approved := true
	msgs := []provider.Message{
		provider.UserText("please transfer"),
		provider.NewAssistantMessage(
			provider.ToolCallPart("c1", "dangerous", json.RawMessage(`{}`)),
			provider.ToolApprovalRequestPart("apr_1", "c1", false),
		),
		provider.NewToolMessage(
			provider.ToolApprovalResponsePart("apr_1", approved, "ok"),
			provider.ToolResultPart("c1", "dangerous", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"already":true}`)}),
		),
	}
	var capturedPrompt []provider.Message
	var executeCount atomic.Int32
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		capturedPrompt = opts.Prompt
		return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(msgs...),
		WithTools(ToolSet{"dangerous": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executeCount.Add(1)
			return json.RawMessage(`{}`), nil
		}}}),
	)
	for range result.FullStream() {
	}

	assert.Equal(t, int32(0), executeCount.Load())
	require.NotEmpty(t, capturedPrompt)
	last := capturedPrompt[len(capturedPrompt)-1]
	require.Equal(t, provider.RoleTool, last.Role)
	require.Len(t, last.Content, 1)
	assert.Equal(t, provider.ContentPartTypeToolResult, last.Content[0].Type)
}

func TestStreamTextToolApproval_InvalidReferences(t *testing.T) {
	t.Run("unknown approval id", func(t *testing.T) {
		approved := true
		model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("should not run")}, nil
		}}
		result := StreamText(context.Background(), model,
			WithModelMessages(
				provider.UserText("hi"),
				provider.NewToolMessage(provider.ToolApprovalResponsePart("missing", approved, "")),
			),
		)
		for range result.FullStream() {
		}
		require.Error(t, result.Err())
		var invalidErr *InvalidToolApprovalError
		require.ErrorAs(t, result.Err(), &invalidErr)
		assert.Equal(t, "missing", invalidErr.ApprovalID)
		assert.Empty(t, invalidErr.Reason, "no matching request: Reason should be empty")
		assert.Equal(t, 0, model.callCount)
	})

	t.Run("missing tool call", func(t *testing.T) {
		approved := true
		model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("should not run")}, nil
		}}
		result := StreamText(context.Background(), model,
			WithModelMessages(
				provider.UserText("hi"),
				provider.NewAssistantMessage(provider.ToolApprovalRequestPart("apr_1", "c1", false)),
				provider.NewToolMessage(provider.ToolApprovalResponsePart("apr_1", approved, "")),
			),
		)
		for range result.FullStream() {
		}
		require.Error(t, result.Err())
		var notFoundErr *ToolCallNotFoundForApprovalError
		require.ErrorAs(t, result.Err(), &notFoundErr)
		assert.Equal(t, "c1", notFoundErr.ToolCallID)
		assert.Equal(t, "apr_1", notFoundErr.ApprovalID)
		assert.Equal(t, 0, model.callCount)
	})

	t.Run("missing approved field", func(t *testing.T) {
		model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("should not run")}, nil
		}}
		result := StreamText(context.Background(), model,
			WithModelMessages(
				provider.UserText("hi"),
				provider.NewAssistantMessage(
					provider.ToolCallPart("c1", "dangerous", json.RawMessage(`{}`)),
					provider.ToolApprovalRequestPart("apr_1", "c1", false),
				),
				provider.NewToolMessage(provider.ContentPart{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "apr_1"}),
			),
		)
		for range result.FullStream() {
		}
		require.Error(t, result.Err())
		var invalidErr *InvalidToolApprovalError
		require.ErrorAs(t, result.Err(), &invalidErr)
		assert.Equal(t, "apr_1", invalidErr.ApprovalID)
		assert.Equal(t, "approved is required", invalidErr.Reason)
		assert.Equal(t, 0, model.callCount)
	})

	t.Run("approved tool without execute", func(t *testing.T) {
		approved := true
		model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("should not run")}, nil
		}}
		result := StreamText(context.Background(), model,
			WithModelMessages(
				provider.UserText("hi"),
				provider.NewAssistantMessage(
					provider.ToolCallPart("c1", "dangerous", json.RawMessage(`{}`)),
					provider.ToolApprovalRequestPart("apr_1", "c1", false),
				),
				provider.NewToolMessage(provider.ToolApprovalResponsePart("apr_1", approved, "")),
			),
			// No Execute func: resume cannot run the approved tool.
			WithTools(ToolSet{"dangerous": Tool{}}),
		)
		for range result.FullStream() {
		}
		require.Error(t, result.Err())
		var notExecErr *ToolNotExecutableError
		require.ErrorAs(t, result.Err(), &notExecErr)
		assert.Equal(t, "dangerous", notExecErr.ToolName)
		assert.Equal(t, 0, model.callCount)
	})
}

func TestStreamTextToolApproval_CallLevelPolicyPrecedence(t *testing.T) {
	var executeCount atomic.Int32
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: toolCallStreamParts("dangerous", `{}`)}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("do it")),
		WithToolApproval(ToolApprovalMap{"dangerous": ApprovalPolicy(ToolApprovalNotApplicable)}),
		WithTools(ToolSet{"dangerous": Tool{NeedsApproval: ApprovalRequired(), Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executeCount.Add(1)
			return json.RawMessage(`{"ok":true}`), nil
		}}}),
	)
	for range result.FullStream() {
	}

	assert.Equal(t, int32(1), executeCount.Load())
	steps := result.Steps()
	require.Len(t, steps, 1)
	assert.Len(t, steps[0].ToolApprovalRequests, 0)
}

func TestStreamTextToolApproval_GenericPolicy(t *testing.T) {
	var called bool
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: toolCallStreamParts("dangerous", `{}`)}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("do it")),
		WithGenerateID(func() string { return "apr_1" }),
		WithToolApproval(ToolApprovalFunc(func(opts ToolApprovalOptions) (ToolApprovalDecision, error) {
			called = true
			assert.Equal(t, "dangerous", opts.ToolCall.ToolName)
			assert.Contains(t, opts.Tools, "dangerous")
			assert.NotEmpty(t, opts.Messages)
			return ToolApprovalDecision{Status: ToolApprovalUserApproval}, nil
		})),
		WithTools(ToolSet{"dangerous": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`{}`), nil
		}}}),
	)
	for range result.FullStream() {
	}

	assert.True(t, called)
	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].ToolApprovalRequests, 1)
	assert.Equal(t, "apr_1", steps[0].ToolApprovalRequests[0].ApprovalID)
}

func TestStreamTextToolApproval_WithToolApprovalLastCallWins(t *testing.T) {
	var executeCount atomic.Int32
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: toolCallStreamParts("dangerous", `{}`)}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("do it")),
		WithToolApproval(ToolApprovalFunc(func(ToolApprovalOptions) (ToolApprovalDecision, error) {
			return ToolApprovalDecision{Status: ToolApprovalUserApproval}, nil
		})),
		WithToolApproval(ToolApprovalMap{"dangerous": ApprovalPolicy(ToolApprovalNotApplicable)}),
		WithTools(ToolSet{"dangerous": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executeCount.Add(1)
			return json.RawMessage(`{"ok":true}`), nil
		}}}),
	)
	for range result.FullStream() {
	}

	assert.Equal(t, int32(1), executeCount.Load())
	steps := result.Steps()
	require.Len(t, steps, 1)
	assert.Empty(t, steps[0].ToolApprovalRequests)
}

func TestStreamTextToolApproval_AutomaticApprovedPolicy(t *testing.T) {
	var executeCount atomic.Int32
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: toolCallStreamParts("safe", `{}`)}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("do it")),
		WithGenerateID(func() string { return "apr_1" }),
		WithToolApproval(ToolApprovalMap{"safe": ApprovalPolicy(ToolApprovalApproved, "policy approved")}),
		WithTools(ToolSet{"safe": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executeCount.Add(1)
			return json.RawMessage(`{"ok":true}`), nil
		}}}),
	)
	var sawResponse bool
	for part := range result.FullStream() {
		if p, ok := part.(StreamToolApprovalResponse); ok {
			sawResponse = true
			assert.True(t, p.Approved)
			assert.Equal(t, "policy approved", p.Reason)
		}
	}

	assert.True(t, sawResponse)
	assert.Equal(t, int32(1), executeCount.Load())
	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].ToolApprovalRequests, 1)
	require.Len(t, steps[0].ToolApprovalResponses, 1)
	assert.True(t, steps[0].ToolApprovalRequests[0].IsAutomatic)
	assert.Len(t, steps[0].ToolResults, 1)
}

func TestStreamTextToolApproval_ProviderPromptStripsApprovalRequests(t *testing.T) {
	callNum := 0
	var capturedPrompt []provider.Message
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		callNum++
		if callNum == 1 {
			return &provider.StreamResult{Stream: toolCallStreamParts("safe", `{}`)}, nil
		}
		capturedPrompt = opts.Prompt
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("do it")),
		WithGenerateID(func() string { return "apr_1" }),
		WithToolApproval(ToolApprovalMap{"safe": ApprovalPolicy(ToolApprovalApproved, "policy approved")}),
		WithTools(ToolSet{"safe": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		}}}),
		WithStopWhen(StepCountIs(2)),
	)
	for range result.FullStream() {
	}

	require.Equal(t, 2, callNum)
	require.NotEmpty(t, capturedPrompt)
	for _, msg := range capturedPrompt {
		for _, part := range msg.Content {
			assert.NotEqual(t, provider.ContentPartTypeToolApprovalRequest, part.Type)
		}
	}
}

func TestStreamTextToolApproval_AutomaticDeniedPolicy(t *testing.T) {
	var executeCount atomic.Int32
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: toolCallStreamParts("dangerous", `{}`)}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("do it")),
		WithGenerateID(func() string { return "apr_1" }),
		WithToolApproval(ToolApprovalMap{"dangerous": ApprovalPolicy(ToolApprovalDenied, "policy denied")}),
		WithTools(ToolSet{"dangerous": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executeCount.Add(1)
			return json.RawMessage(`{}`), nil
		}}}),
	)
	var sawDenied bool
	var sawResponse bool
	for part := range result.FullStream() {
		if p, ok := part.(StreamToolApprovalResponse); ok {
			sawResponse = true
			assert.False(t, p.Approved)
			assert.Equal(t, "policy denied", p.Reason)
		}
		if _, ok := part.(StreamToolOutputDenied); ok {
			sawDenied = true
		}
	}

	assert.True(t, sawResponse)
	assert.False(t, sawDenied)
	assert.Equal(t, int32(0), executeCount.Load())
	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].ToolApprovalResponses, 1)
	assert.False(t, steps[0].ToolApprovalResponses[0].Approved)
	assert.Equal(t, "policy denied", steps[0].ToolApprovalResponses[0].Reason)
	require.Len(t, steps[0].ToolResults, 1)
	require.NotNil(t, steps[0].ToolResults[0].ModelOutput)
	assert.Equal(t, provider.ToolOutputExecutionDenied, steps[0].ToolResults[0].ModelOutput.Type)
	assert.Equal(t, "policy denied", steps[0].ToolResults[0].ModelOutput.Reason)
}

func TestStreamTextDefaultStopCondition(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("weather", `{}`)}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{
			"weather": Tool{
				Description: "Get weather",
				Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{}`), nil
				},
			},
		}),
	)

	for range result.FullStream() {
	}

	assert.Len(t, result.Steps(), 1, "default StepCountIs(1) should limit to 1 step")
}

func TestStreamTextContextCancellation(t *testing.T) {
	tests := []struct {
		name            string
		providerParts   []provider.StreamPart
		waitForDelta    bool
		pendingAtCancel bool
		expectedTypes   []string
	}{
		{
			name:          "no output",
			expectedTypes: []string{"start", "start-step", "abort"},
		},
		{
			name: "partial output",
			providerParts: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "t1"},
				{Type: provider.PartTextDelta, ID: "t1", Delta: "partial"},
			},
			waitForDelta:  true,
			expectedTypes: []string{"start", "start-step", "text-start", "text-delta", "abort"},
		},
		{
			name: "pending output at cancellation",
			providerParts: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "t1"},
				{Type: provider.PartTextDelta, ID: "t1", Delta: "discarded"},
			},
			pendingAtCancel: true,
			expectedTypes:   []string{"start", "start-step", "abort"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			streamBuffer := 0
			if tc.pendingAtCancel {
				streamBuffer = len(tc.providerParts)
			}
			stream := make(chan provider.StreamPart, streamBuffer)
			started := make(chan struct{})
			published := make(chan struct{})
			release := make(chan struct{})
			model := &mockModel{
				streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					close(started)
					if tc.pendingAtCancel {
						<-release
					} else {
						go func() {
							for _, part := range tc.providerParts {
								stream <- part
							}
							close(published)
						}()
					}
					return &provider.StreamResult{Stream: stream}, nil
				},
			}

			var abortCalls int
			var abortChunkCalls int
			var errorCalls int
			deltaEmitted := make(chan struct{})
			result := StreamText(ctx, model,
				WithModelMessages(provider.UserText("hi")),
				OnAbort(func(OnAbortState) { abortCalls++ }),
				OnChunk(func(state OnChunkState) {
					switch state.Chunk.(type) {
					case StreamAbort:
						abortChunkCalls++
					case StreamTextDelta:
						if tc.waitForDelta {
							close(deltaEmitted)
						}
					}
				}),
				OnError(func(error) { errorCalls++ }),
			)

			<-started
			if tc.pendingAtCancel {
				for _, part := range tc.providerParts {
					stream <- part
				}
				cancel(fmt.Errorf("manual abort"))
				close(release)
			} else {
				<-published
				if tc.waitForDelta {
					<-deltaEmitted
				}
				cancel(fmt.Errorf("manual abort"))
			}

			var parts []TextStreamPart
			for part := range result.FullStream() {
				parts = append(parts, part)
			}

			types := make([]string, 0, len(parts))
			var aborts []StreamAbort
			for _, part := range parts {
				types = append(types, typeName(part))
				if abort, ok := part.(StreamAbort); ok {
					aborts = append(aborts, abort)
				}
			}
			assert.Equal(t, tc.expectedTypes, types)
			require.Len(t, aborts, 1)
			assert.Equal(t, "manual abort", aborts[0].Reason)
			assert.Equal(t, 1, abortCalls)
			assert.Equal(t, 1, abortChunkCalls)
			assert.Zero(t, errorCalls)
			assert.NoError(t, result.Err())
		})
	}
}

func TestStreamTextCallbacks(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("hi")}, nil
		},
	}

	var startCalled, stepStartCalled, stepFinishCalled bool
	var chunkCount int

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		OnStart(func(_ OnStartState) { startCalled = true }),
		OnStepStart(func(_ OnStepStartState) { stepStartCalled = true }),
		OnStepFinish(func(_ OnStepFinishState) { stepFinishCalled = true }),
		OnChunk(func(_ OnChunkState) { chunkCount++ }),
	)

	for range result.FullStream() {
	}

	assert.True(t, startCalled, "OnStart not called")
	assert.True(t, stepStartCalled, "OnStepStart not called")
	assert.True(t, stepFinishCalled, "OnStepFinish not called")
	assert.Greater(t, chunkCount, 0, "OnChunk not called")
}

func TestStreamTextPrepareStepNumber(t *testing.T) {
	callNum := 0
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			callNum++
			if callNum == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("next", `{}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		},
	}

	var stepNumbers []int
	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{
			"next": Tool{
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`"ok"`), nil
				},
			},
		}),
		WithStopWhen(StepCountIs(2)),
		WithPrepareStep(func(state PrepareStepState) (*PrepareStepResult, error) {
			stepNumbers = append(stepNumbers, state.StepNumber)
			assert.Len(t, state.Steps, state.StepNumber)
			return nil, nil
		}),
	)

	for range result.FullStream() {
	}

	assert.Equal(t, []int{0, 1}, stepNumbers)
	assert.Len(t, result.Steps(), 2)
	assert.Equal(t, 2, model.callCount)
}

func TestStreamTextOnStepEndCallback(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("hi")}, nil
		},
	}

	var stepEndCalled bool
	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		OnStepEnd(func(_ OnStepFinishState) { stepEndCalled = true }),
	)

	for range result.FullStream() {
	}

	assert.True(t, stepEndCalled, "OnStepEnd not called")
}

func TestStreamTextTotalUsage(t *testing.T) {
	callNum := 0
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			callNum++
			if callNum == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("w", `{}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{
			"w": Tool{
				Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{}`), nil
				},
			},
		}),
		WithStopWhen(StepCountIs(5)),
	)

	for range result.FullStream() {
	}

	tu := result.Usage()
	require.NotNil(t, tu.InputTokens.Total)
	assert.Equal(t, 20, *tu.InputTokens.Total)
	require.NotNil(t, tu.OutputTokens.Total)
	assert.Equal(t, 8, *tu.OutputTokens.Total)

	assert.Equal(t, result.TotalUsage(), result.AggregateUsage())
}

func TestStreamTextToUIMessageStream(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("hello")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
	)

	var chunkTypes []ChunkType
	for chunk := range result.ToUIMessageStream() {
		chunkTypes = append(chunkTypes, chunk.Type)
	}

	require.GreaterOrEqual(t, len(chunkTypes), 5)
	assert.Contains(t, chunkTypes, ChunkTextDelta)
}

func TestStreamTextToolCallBecomesToolInputAvailable(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("ext", `{"q":"x"}`)}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{"ext": Tool{}}),
		WithStopWhen(StepCountIs(5)),
	)

	var found bool
	for chunk := range result.ToUIMessageStream() {
		if chunk.Type == ChunkToolInputAvailable {
			found = true
			assert.Equal(t, "ext", chunk.ToolName)
		}
	}
	assert.True(t, found, "expected tool-input-available chunk")
}

func TestGenerateText(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: textStreamParts("hello")}, nil
			},
		}

		gen, err := GenerateText(context.Background(), model,
			WithModelMessages(provider.UserText("hi")),
		)
		require.NoError(t, err)
		assert.Equal(t, "hello", gen.Text)
		assert.Equal(t, provider.FinishReasonStop, gen.FinishReason.Unified)
	})

	t.Run("rejects unresolved tool calls before model call", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStreamParts("should not run")}, nil
		}}

		_, err := GenerateText(context.Background(), model,
			WithModelMessages(provider.NewAssistantMessage(
				provider.ToolCallPart("call-missing", "regular_tool", json.RawMessage(`{}`)),
			)),
		)

		var missingErr *MissingToolResultsError
		require.ErrorAs(t, err, &missingErr)
		assert.Equal(t, []string{"call-missing"}, missingErr.ToolCallIDs)
		assert.Equal(t, 0, model.callCount)
	})

	t.Run("multi-step", func(t *testing.T) {
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: toolCallStreamParts("calc", `{"x":2}`)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("result is 4")}, nil
			},
		}

		gen, err := GenerateText(context.Background(), model,
			WithModelMessages(provider.UserText("2*2")),
			WithTools(ToolSet{
				"calc": Tool{
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return json.RawMessage(`{"result":4}`), nil
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)
		require.NoError(t, err)
		assert.Equal(t, "result is 4", gen.Text)
		assert.Len(t, gen.Steps, 2)
	})

	t.Run("returns cancellation during tool execution", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(t.Context())
		cancelErr := errors.New("tool execution canceled")
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: toolCallStreamParts("cancel", `{}`)}, nil
			},
		}

		_, err := GenerateText(ctx, model,
			WithModelMessages(provider.UserText("cancel")),
			WithTools(ToolSet{"cancel": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				cancel(cancelErr)
				return nil, context.Cause(ctx)
			}}}),
			WithStopWhen(StepCountIs(5)),
		)

		require.ErrorIs(t, err, cancelErr)
		assert.Equal(t, 1, model.callCount)
	})

	t.Run("returns cancellation after tool execution before another model call", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(t.Context())
		cancelErr := errors.New("tool execution canceled")
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: toolCallStreamParts("cancel", `{}`)}, nil
			},
		}

		_, err := GenerateText(ctx, model,
			WithModelMessages(provider.UserText("cancel")),
			WithTools(ToolSet{"cancel": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				cancel(cancelErr)
				return json.RawMessage(`{"done":true}`), nil
			}}}),
			WithStopWhen(StepCountIs(5)),
		)

		require.ErrorIs(t, err, cancelErr)
		assert.Equal(t, 1, model.callCount)
	})

	t.Run("approval request", func(t *testing.T) {
		model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("dangerous", `{}`)}, nil
		}}
		gen, err := GenerateText(context.Background(), model,
			WithModelMessages(provider.UserText("transfer")),
			WithGenerateID(func() string { return "apr_1" }),
			WithTools(ToolSet{"dangerous": Tool{NeedsApproval: ApprovalRequired(), Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				return json.RawMessage(`{}`), nil
			}}}),
			WithStopWhen(StepCountIs(5)),
		)
		require.NoError(t, err)
		require.Len(t, gen.Content, 2)
		_, ok := gen.Content[1].(ToolApprovalRequestContent)
		assert.True(t, ok)
		require.Len(t, gen.Steps, 1)
		assert.Len(t, gen.Steps[0].ToolResults, 0)
	})

	t.Run("approval resumption", func(t *testing.T) {
		approved := true
		var capturedPrompt []provider.Message
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			capturedPrompt = opts.Prompt
			return &provider.StreamResult{Stream: textStreamParts("approved")}, nil
		}}
		gen, err := GenerateText(context.Background(), model,
			WithModelMessages(
				provider.UserText("transfer"),
				provider.NewAssistantMessage(
					provider.ToolCallPart("c1", "dangerous", json.RawMessage(`{}`)),
					provider.ToolApprovalRequestPart("apr_1", "c1", false),
				),
				provider.NewToolMessage(provider.ToolApprovalResponsePart("apr_1", approved, "ok")),
			),
			WithTools(ToolSet{"dangerous": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				return json.RawMessage(`{"ok":true}`), nil
			}}}),
		)
		require.NoError(t, err)
		assert.Equal(t, "approved", gen.Text)
		require.NotEmpty(t, capturedPrompt)
		last := capturedPrompt[len(capturedPrompt)-1]
		require.Equal(t, provider.RoleTool, last.Role)
		require.Len(t, last.Content, 1)
		assert.Equal(t, provider.ContentPartTypeToolResult, last.Content[0].Type)
	})

	t.Run("returns error from stream", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, 5)
				go func() {
					defer close(ch)
					ch <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "provider exploded"})}
					ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonError}}
				}()
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		_, err := GenerateText(context.Background(), model,
			WithModelMessages(provider.UserText("hi")),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider exploded")
	})

	t.Run("returns error from DoStream", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return nil, fmt.Errorf("connection refused")
			},
		}

		_, err := GenerateText(context.Background(), model,
			WithModelMessages(provider.UserText("hi")),
		)
		require.Error(t, err)
		assert.Equal(t, "connection refused", err.Error())
	})

	// Guards against a regression where a malformed PartError stream part
	// (Type=error but APICallError nil) was silently dropped, leaving the
	// generation appearing successful. The orchestration layer must
	// synthesize an APICallError so consumers always observe the failure.
	t.Run("synthesizes APICallError when PartError carries nil APICallError", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, 5)
				go func() {
					defer close(ch)
					// APICallError intentionally nil to simulate a buggy producer.
					ch <- provider.StreamPart{Type: provider.PartError}
					ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonError}}
				}()
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		_, err := GenerateText(context.Background(), model,
			WithModelMessages(provider.UserText("hi")),
		)
		require.Error(t, err)
		var apiErr *provider.APICallError
		require.ErrorAs(t, err, &apiErr, "synthesized error must be an *APICallError")
		assert.Contains(t, apiErr.Error(), "without APICallError details")
	})
}

func TestStreamTextToolInputCallbackOrder(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		ch := make(chan provider.StreamPart, 5)
		ch <- provider.StreamPart{Type: provider.PartToolInputStart, ID: "call_1", ToolName: "browser"}
		ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call_1", Delta: `{"actions":[]}`}
		ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call_1"}
		ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call_1", ToolName: "browser", Input: `{"actions":[]}`}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
		close(ch)
		return &provider.StreamResult{Stream: ch}, nil
	}}
	var callbacks []string
	result := StreamText(context.Background(), model, WithTools(ToolSet{
		"browser": {
			OnInputStart: func(ToolExecutionOptions) {
				callbacks = append(callbacks, "start")
			},
			OnInputDelta: func(string, ToolExecutionOptions) {
				callbacks = append(callbacks, "delta")
			},
			OnInputAvailable: func(json.RawMessage, ToolExecutionOptions) {
				callbacks = append(callbacks, "available")
			},
		},
	}))
	for range result.FullStream() {
	}

	assert.Equal(t, []string{"start", "delta", "available"}, callbacks)
}

func TestStreamTextActiveToolsFiltering(t *testing.T) {
	var receivedTools []provider.Tool
	model := &mockModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			receivedTools = opts.Tools
			return &provider.StreamResult{Stream: textStreamParts("ok")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{
			"search":     Tool{Description: "search"},
			"calculator": Tool{Description: "calc"},
			"weather":    Tool{Description: "weather"},
		}),
		WithActiveTools("search", "weather"),
	)

	for range result.FullStream() {
	}

	require.Len(t, receivedTools, 2)
	names := map[string]bool{}
	for _, tool := range receivedTools {
		names[tool.Name] = true
	}
	assert.True(t, names["search"])
	assert.True(t, names["weather"])
}

func TestStreamTextResponseAndProviderMetadata(t *testing.T) {
	textMetadata := provider.ProviderMetadata{"anthropic": json.RawMessage(`{"citations":[{"type":"web_search_result_location"}]}`)}
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 10)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{
					Type:     provider.PartStreamStart,
					Warnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: "logprobs"}},
				}
				ch <- provider.StreamPart{
					Type:            provider.PartResponseMeta,
					ResponseID:      "resp-123",
					ModelID:         "gpt-4",
					ResponseHeaders: map[string]string{"x-req-id": "abc"},
				}
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
				ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "ok"}
				ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1", ProviderMetadata: textMetadata}
				ch <- provider.StreamPart{
					Type:             provider.PartFinish,
					FinishReason:     &provider.FinishReason{Unified: provider.FinishReasonStop},
					Usage:            &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(5)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(3)}},
					ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"foo":"bar"}`)},
				}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
	)
	for range result.FullStream() {
	}

	resp := result.Response()
	assert.Equal(t, "resp-123", resp.ID)
	assert.Equal(t, "gpt-4", resp.ModelID)
	assert.Equal(t, "abc", resp.Headers["x-req-id"])

	pm := result.ProviderMetadata()
	require.NotNil(t, pm)
	assert.JSONEq(t, `{"foo":"bar"}`, string(pm["openai"]))

	warnings := result.Warnings()
	require.Len(t, warnings, 1)
	assert.Equal(t, "logprobs", warnings[0].Feature)

	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].responseContent, 1)
	storedMetadata := optionsToProviderMetadata(steps[0].responseContent[0].ProviderOptions)
	assert.JSONEq(t, string(textMetadata["anthropic"]), string(storedMetadata["anthropic"]))
}

func TestStreamTextRawFinishReasonPropagation(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 5)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
				ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "done"}
				ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
				ch <- provider.StreamPart{
					Type:         provider.PartFinish,
					FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "stop_sequence"},
					Usage:        &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(1)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(1)}},
				}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
	)
	for range result.FullStream() {
	}

	steps := result.Steps()
	require.Len(t, steps, 1)
	assert.Equal(t, "stop_sequence", steps[0].FinishReason.Raw)
}

func TestStreamTextToolInputDeltaNameTracking(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 10)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{
					Type: provider.PartToolInputStart, ID: "c1",
					ToolName: "weather", Title: "Get Weather",
				}
				ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: "c1", Delta: `{"city":`}
				ch <- provider.StreamPart{Type: provider.PartToolInputDelta, ID: "c1", Delta: `"NYC"}`}
				ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: "c1"}
				ch <- provider.StreamPart{
					Type: provider.PartToolCall, ToolCallID: "c1",
					ToolName: "weather", Input: `{"city":"NYC"}`,
				}
				ch <- provider.StreamPart{
					Type:         provider.PartFinish,
					FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls},
					Usage:        &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(5)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(2)}},
				}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	var inputDeltaCalled int
	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("weather?")),
		WithTools(ToolSet{
			"weather": Tool{
				Description: "Get weather",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				OnInputDelta: func(delta string, opts ToolExecutionOptions) {
					inputDeltaCalled++
				},
			},
		}),
	)

	for range result.FullStream() {
	}

	assert.Equal(t, 2, inputDeltaCalled)

	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].ToolCalls, 1)
	assert.Equal(t, "Get Weather", steps[0].ToolCalls[0].Title)
}

func TestStreamTextTitleTrackingFromInputStart(t *testing.T) {
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 10)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{
					Type: provider.PartToolInputStart, ID: "c1",
					ToolName: "search", Title: "Web Search",
				}
				ch <- provider.StreamPart{Type: provider.PartToolInputEnd, ID: "c1"}
				ch <- provider.StreamPart{
					Type: provider.PartToolCall, ToolCallID: "c1",
					ToolName: "search", Input: `{"q":"test"}`,
				}
				ch <- provider.StreamPart{
					Type:         provider.PartFinish,
					FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls},
					Usage:        &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(3)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(1)}},
				}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("search")),
		WithTools(ToolSet{"search": Tool{Description: "search"}}),
	)
	for range result.FullStream() {
	}

	steps := result.Steps()
	require.Len(t, steps[0].ToolCalls, 1)
	assert.Equal(t, "Web Search", steps[0].ToolCalls[0].Title)
}

func TestStreamTextContextCancellationInStep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	slowCh := make(chan provider.StreamPart)
	model := &mockModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: slowCh}, nil
		},
	}

	var executed atomic.Int32
	result := StreamText(ctx, model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{"weather": Tool{Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			executed.Add(1)
			return json.RawMessage(`{}`), nil
		}}}),
	)

	slowCh <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "weather", Input: `{}`}
	cancel()

	var finishStepCount int
	for part := range result.FullStream() {
		if _, ok := part.(StreamFinishStep); ok {
			finishStepCount++
		}
	}

	assert.Equal(t, 0, finishStepCount)
	assert.Empty(t, result.Steps())
	assert.Equal(t, int32(0), executed.Load())
}

func TestStreamTextAbortCallbacks(t *testing.T) {
	t.Run("MidStreamCancel_NoCompletedStep", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		slowCh := make(chan provider.StreamPart)
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: slowCh}, nil
			},
		}

		var abortCalled bool
		var abortSteps []StepResult
		var finishCalled bool

		result := StreamText(ctx, model,
			WithModelMessages(provider.UserText("hi")),
			OnAbort(func(s OnAbortState) {
				abortCalled = true
				abortSteps = s.Steps
			}),
			OnFinish(func(_ OnFinishState) {
				finishCalled = true
			}),
		)

		slowCh <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		slowCh <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "partial"}
		cancel()

		for range result.FullStream() {
		}

		assert.True(t, abortCalled, "OnAbort should fire on mid-stream cancel")
		assert.Empty(t, abortSteps, "no completed steps should be reported")
		assert.False(t, finishCalled, "OnFinish should not fire with no completed steps")
	})

	t.Run("MidStreamCancel_AfterCompletedStep", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		callNum := 0
		slowCh := make(chan provider.StreamPart)
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: toolCallStreamParts("w", `{}`)}, nil
				}
				return &provider.StreamResult{Stream: slowCh}, nil
			},
		}

		var abortCalled bool
		var abortSteps []StepResult
		var finishCalled bool
		var finishSteps []StepResult

		result := StreamText(ctx, model,
			WithModelMessages(provider.UserText("hi")),
			WithTools(ToolSet{
				"w": Tool{
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return json.RawMessage(`{}`), nil
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
			OnAbort(func(s OnAbortState) {
				abortCalled = true
				abortSteps = s.Steps
			}),
			OnFinish(func(s OnFinishState) {
				finishCalled = true
				finishSteps = s.Steps
			}),
		)

		slowCh <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		cancel()

		for range result.FullStream() {
		}

		assert.True(t, abortCalled, "OnAbort should fire on mid-stream cancel")
		assert.Len(t, abortSteps, 1, "abort should see the completed first step")
		assert.True(t, finishCalled, "OnFinish should fire when completed steps exist")
		assert.Len(t, finishSteps, 1, "OnFinish should see only completed steps")
	})

	t.Run("NormalCompletion_NoAbort", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			},
		}

		var abortCalled bool
		var finishCalled bool
		var finishSteps []StepResult

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("hi")),
			OnAbort(func(_ OnAbortState) {
				abortCalled = true
			}),
			OnFinish(func(s OnFinishState) {
				finishCalled = true
				finishSteps = s.Steps
			}),
		)

		for range result.FullStream() {
		}

		assert.False(t, abortCalled, "OnAbort should not fire on normal completion")
		assert.True(t, finishCalled, "OnFinish should fire on normal completion")
		assert.Len(t, finishSteps, 1, "OnFinish should see the completed step")
	})
}

func TestStopConditions(t *testing.T) {
	t.Run("StepCountIs", func(t *testing.T) {
		cond := StepCountIs(3)
		assert.False(t, cond(StopConditionState{Steps: make([]StepResult, 2)}), "should not stop at 2 steps")
		assert.True(t, cond(StopConditionState{Steps: make([]StepResult, 3)}), "should stop at 3 steps")
	})

	t.Run("HasToolCall", func(t *testing.T) {
		cond := HasToolCall("finalAnswer")
		assert.False(t, cond(StopConditionState{}), "should not match empty steps")

		steps := []StepResult{{ToolCalls: []ToolCall{{ToolName: "weather"}}}}
		assert.False(t, cond(StopConditionState{Steps: steps}), "should not match wrong tool")

		steps = []StepResult{{ToolCalls: []ToolCall{{ToolName: "finalAnswer"}}}}
		assert.True(t, cond(StopConditionState{Steps: steps}), "should match finalAnswer")
	})
}

func TestStreamTextToolErrorProducesToolResult(t *testing.T) {
	callNum := 0
	dynamic := true
	metadata := provider.ProviderMetadata{"test": json.RawMessage(`{"itemId":"call-1"}`)}
	model := &mockModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			callNum++
			if callNum == 1 {
				stream := make(chan provider.StreamPart, 2)
				stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "flaky", Input: `{"x":1}`, Dynamic: &dynamic, ProviderMetadata: metadata}
				stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
				close(stream)
				return &provider.StreamResult{Stream: stream}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("recovered")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("hi")),
		WithTools(ToolSet{
			"flaky": Tool{
				Type:        UserToolDynamic,
				Description: "Flaky tool",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return nil, fmt.Errorf("transient failure")
				},
			},
		}),
		WithStopWhen(StepCountIs(5)),
	)

	for range result.FullStream() {
	}

	steps := result.Steps()
	require.GreaterOrEqual(t, len(steps), 2, "error result should allow continuation")
	require.Len(t, steps[0].ToolResults, 1)

	tr := steps[0].ToolResults[0]
	require.NotNil(t, tr.ModelOutput)
	assert.Equal(t, provider.ToolOutputErrorText, tr.ModelOutput.Type)
	assert.Equal(t, "transient failure", tr.ModelOutput.Text)
	assert.Equal(t, boolPtr(true), tr.Dynamic)
	assert.Equal(t, metadata, tr.ProviderMetadata)
	assert.True(t, tr.IsError)
	require.Error(t, tr.Error)
	require.Len(t, steps[0].Content, 2)
	toolError, ok := steps[0].Content[1].(ToolErrorContent)
	require.True(t, ok)
	_, ok = toolError.Error.(error)
	assert.True(t, ok)
	assert.Equal(t, boolPtr(true), toolError.Dynamic)
	assert.Equal(t, metadata, toolError.ProviderMetadata)
	assert.Equal(t, "recovered", result.Text())
}

func TestStreamTextUnavailableToolRejection(t *testing.T) {
	callNum := 0
	model := &mockModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			callNum++
			if callNum == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("unknown_tool", `{"q":"test"}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("recovered")}, nil
		},
	}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("go")),
		WithTools(ToolSet{
			"known_tool": Tool{
				Description: "A known tool",
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`"ok"`), nil
				},
			},
		}),
		WithStopWhen(StepCountIs(5)),
	)

	var chunks []UIMessageChunk
	for part := range result.FullStream() {
		chunks = append(chunks, translateToChunks(part, uiMessageStreamConfig{})...)
	}

	var inputErrors, outputErrors []UIMessageChunk
	for _, c := range chunks {
		switch c.Type {
		case ChunkToolInputError:
			inputErrors = append(inputErrors, c)
		case ChunkToolOutputError:
			outputErrors = append(outputErrors, c)
		}
	}

	require.Len(t, inputErrors, 1, "should emit exactly one tool-input-error")
	require.Len(t, outputErrors, 1, "should emit exactly one tool-output-error")

	ie := inputErrors[0]
	assert.Equal(t, "c1", ie.ToolCallID)
	assert.Equal(t, "unknown_tool", ie.ToolName)
	assert.Equal(t, json.RawMessage(`{"q":"test"}`), ie.Input)
	assert.Equal(t, boolPtr(true), ie.Dynamic)
	assert.Equal(t, "An error occurred.", ie.ErrorText)

	oe := outputErrors[0]
	assert.Equal(t, "c1", oe.ToolCallID)
	assert.Equal(t, boolPtr(true), oe.Dynamic)
	assert.Equal(t, "An error occurred.", oe.ErrorText)

	steps := result.Steps()
	require.GreaterOrEqual(t, len(steps), 2, "should continue to next step after unavailable tool error")

	require.Len(t, steps[0].ToolCalls, 1, "unavailable tool call should be tracked in step")
	tc := steps[0].ToolCalls[0]
	assert.Equal(t, "unknown_tool", tc.ToolName)
	assert.True(t, tc.Invalid, "tool call should be marked invalid")
	assert.Equal(t, boolPtr(true), tc.Dynamic)

	require.Len(t, steps[0].ToolResults, 1, "unavailable tool result should be tracked in step")
	tr := steps[0].ToolResults[0]
	assert.Equal(t, "unknown_tool", tr.ToolName)
	require.NotNil(t, tr.ModelOutput)
	assert.Equal(t, provider.ToolOutputErrorText, tr.ModelOutput.Type)
	assert.Contains(t, tr.ModelOutput.Text, "unavailable tool 'unknown_tool'")

	assert.Equal(t, "recovered", result.Text())
}

func TestTranslateToChunks_ToolFieldPropagation(t *testing.T) {
	meta := provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"echo"}`),
	}
	opts := uiMessageStreamConfig{}

	t.Run("StreamToolCall propagates Dynamic, Title, ProviderMetadata", func(t *testing.T) {
		chunks := translateToChunks(StreamToolCall{
			ToolCallID:       "tc-1",
			ToolName:         "echo",
			Input:            json.RawMessage(`{}`),
			ProviderExecuted: true,
			Dynamic:          boolPtr(true),
			Title:            "Echo Tool",
			ProviderMetadata: meta,
		}, opts)
		require.Len(t, chunks, 1)
		c := chunks[0]
		assert.Equal(t, ChunkToolInputAvailable, c.Type)
		assert.Equal(t, "tc-1", c.ToolCallID)
		assert.Equal(t, boolPtr(true), c.Dynamic)
		assert.Equal(t, "Echo Tool", c.Title)
		assert.Equal(t, meta, c.ProviderMetadata)
	})

	t.Run("StreamToolInputStart propagates Dynamic, Title, ProviderMetadata", func(t *testing.T) {
		chunks := translateToChunks(StreamToolInputStart{
			ID:               "tc-2",
			ToolName:         "search",
			ProviderExecuted: true,
			Dynamic:          boolPtr(true),
			Title:            "Search Tool",
			ProviderMetadata: meta,
		}, opts)
		require.Len(t, chunks, 1)
		c := chunks[0]
		assert.Equal(t, ChunkToolInputStart, c.Type)
		assert.Equal(t, boolPtr(true), c.Dynamic)
		assert.Equal(t, "Search Tool", c.Title)
		assert.Equal(t, meta, c.ProviderMetadata)
	})

	t.Run("StreamToolInputDelta omits ProviderMetadata per upstream schema", func(t *testing.T) {
		chunks := translateToChunks(StreamToolInputDelta{
			ID:    "tc-3",
			Delta: `{"partial":`,
		}, opts)
		require.Len(t, chunks, 1)
		c := chunks[0]
		assert.Equal(t, ChunkToolInputDelta, c.Type)
		assert.Nil(t, c.ProviderMetadata)
	})

	t.Run("StreamToolResult propagates Dynamic, ProviderMetadata", func(t *testing.T) {
		chunks := translateToChunks(StreamToolResult{
			ToolCallID:       "tc-4",
			Output:           json.RawMessage(`"ok"`),
			ProviderExecuted: true,
			Dynamic:          boolPtr(true),
			ProviderMetadata: meta,
		}, opts)
		require.Len(t, chunks, 1)
		c := chunks[0]
		assert.Equal(t, ChunkToolOutputAvailable, c.Type)
		assert.Equal(t, boolPtr(true), c.Dynamic)
		assert.Equal(t, meta, c.ProviderMetadata)
	})

	t.Run("StreamToolError propagates Dynamic, ProviderMetadata", func(t *testing.T) {
		chunks := translateToChunks(StreamToolError{
			ToolCallID:       "tc-5",
			Error:            fmt.Errorf("fail"),
			ProviderExecuted: true,
			Dynamic:          boolPtr(true),
			ProviderMetadata: meta,
		}, opts)
		require.Len(t, chunks, 1)
		c := chunks[0]
		assert.Equal(t, ChunkToolOutputError, c.Type)
		assert.Equal(t, boolPtr(true), c.Dynamic)
		assert.Equal(t, meta, c.ProviderMetadata)
	})

	t.Run("StreamToolResult providerMetadata serializes to JSON", func(t *testing.T) {
		chunks := translateToChunks(StreamToolResult{
			ToolCallID:       "tc-7",
			Output:           json.RawMessage(`"ok"`),
			ProviderExecuted: true,
			Dynamic:          boolPtr(true),
			ProviderMetadata: meta,
		}, opts)
		require.Len(t, chunks, 1)
		b, err := json.Marshal(chunks[0])
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		assert.Equal(t, "tool-output-available", m["type"])
		assert.Equal(t, true, m["dynamic"])
		assert.NotNil(t, m["providerMetadata"], "providerMetadata should appear in JSON")
	})

	t.Run("StreamToolError providerMetadata serializes to JSON", func(t *testing.T) {
		chunks := translateToChunks(StreamToolError{
			ToolCallID:       "tc-8",
			Error:            fmt.Errorf("fail"),
			ProviderExecuted: true,
			Dynamic:          boolPtr(true),
			ProviderMetadata: meta,
		}, opts)
		require.Len(t, chunks, 1)
		b, err := json.Marshal(chunks[0])
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		assert.Equal(t, "tool-output-error", m["type"])
		assert.Equal(t, true, m["dynamic"])
		assert.NotNil(t, m["providerMetadata"], "providerMetadata should appear in JSON")
	})

	t.Run("non-MCP tool chunks have zero-value fields", func(t *testing.T) {
		chunks := translateToChunks(StreamToolCall{
			ToolCallID: "tc-6",
			ToolName:   "local_fn",
			Input:      json.RawMessage(`{"x":1}`),
		}, opts)
		require.Len(t, chunks, 1)
		c := chunks[0]
		assert.Equal(t, ChunkToolInputAvailable, c.Type)
		assert.Nil(t, c.Dynamic)
		assert.Empty(t, c.Title)
		assert.Nil(t, c.ProviderMetadata)

		b, err := json.Marshal(c)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		assert.Nil(t, m["dynamic"], "dynamic should not appear in JSON when false")
		assert.Nil(t, m["title"], "title should not appear in JSON when empty")
		assert.Nil(t, m["providerMetadata"], "providerMetadata should not appear in JSON when nil")
	})
}

func multiToolCallStreamParts(tools ...struct{ name, input string }) <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		for i, t := range tools {
			ch <- provider.StreamPart{
				Type:       provider.PartToolCall,
				ToolCallID: fmt.Sprintf("c%d", i+1),
				ToolName:   t.name,
				Input:      t.input,
			}
		}
		ch <- provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls},
			Usage:        &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(3)}},
		}
	}()
	return ch
}

func TestStreamTextConcurrentToolExecution(t *testing.T) {
	t.Run("concurrent_timing", func(t *testing.T) {
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: multiToolCallStreamParts(
						struct{ name, input string }{"slow_a", `{"id":"a"}`},
						struct{ name, input string }{"slow_b", `{"id":"b"}`},
						struct{ name, input string }{"slow_c", `{"id":"c"}`},
					)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			},
		}

		toolDelay := 100 * time.Millisecond
		makeTool := func(name string) Tool {
			return Tool{
				Description: name,
				InputSchema: testMustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					time.Sleep(toolDelay)
					return json.RawMessage(fmt.Sprintf(`{"tool":%q}`, name)), nil
				},
			}
		}

		start := time.Now()
		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("go")),
			WithTools(ToolSet{
				"slow_a": makeTool("slow_a"),
				"slow_b": makeTool("slow_b"),
				"slow_c": makeTool("slow_c"),
			}),
			WithStopWhen(StepCountIs(5)),
		)

		for range result.FullStream() {
		}
		elapsed := time.Since(start)

		assert.Equal(t, "done", result.Text())

		maxSequential := toolDelay * 3
		assert.Less(t, elapsed, maxSequential, "concurrent execution should be faster than sequential (3 * %v = %v, got %v)", toolDelay, maxSequential, elapsed)
	})

	t.Run("independent_error_handling", func(t *testing.T) {
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: multiToolCallStreamParts(
						struct{ name, input string }{"good_a", `{}`},
						struct{ name, input string }{"bad_b", `{}`},
						struct{ name, input string }{"good_c", `{}`},
					)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("recovered")}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("go")),
			WithTools(ToolSet{
				"good_a": Tool{
					Description: "good_a",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return json.RawMessage(`{"ok":"a"}`), nil
					},
				},
				"bad_b": Tool{
					Description: "bad_b",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return nil, fmt.Errorf("bad_b failed")
					},
				},
				"good_c": Tool{
					Description: "good_c",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return json.RawMessage(`{"ok":"c"}`), nil
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)

		for range result.FullStream() {
		}

		steps := result.Steps()
		require.GreaterOrEqual(t, len(steps), 2)
		require.Len(t, steps[0].ToolResults, 3)

		trA := steps[0].ToolResults[0]
		assert.Equal(t, "good_a", trA.ToolName)
		assert.Equal(t, json.RawMessage(`{"ok":"a"}`), trA.Output)

		trB := steps[0].ToolResults[1]
		assert.Equal(t, "bad_b", trB.ToolName)
		require.NotNil(t, trB.ModelOutput)
		assert.Equal(t, provider.ToolOutputErrorText, trB.ModelOutput.Type)
		assert.Equal(t, "bad_b failed", trB.ModelOutput.Text)

		trC := steps[0].ToolResults[2]
		assert.Equal(t, "good_c", trC.ToolName)
		assert.Equal(t, json.RawMessage(`{"ok":"c"}`), trC.Output)
	})

	t.Run("results_preserve_call_order", func(t *testing.T) {
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: multiToolCallStreamParts(
						struct{ name, input string }{"fast", `{}`},
						struct{ name, input string }{"slow", `{}`},
						struct{ name, input string }{"medium", `{}`},
					)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("go")),
			WithTools(ToolSet{
				"fast": Tool{
					Description: "fast",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						time.Sleep(10 * time.Millisecond)
						return json.RawMessage(`"fast"`), nil
					},
				},
				"slow": Tool{
					Description: "slow",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						time.Sleep(80 * time.Millisecond)
						return json.RawMessage(`"slow"`), nil
					},
				},
				"medium": Tool{
					Description: "medium",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						time.Sleep(40 * time.Millisecond)
						return json.RawMessage(`"medium"`), nil
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)

		for range result.FullStream() {
		}

		steps := result.Steps()
		require.Len(t, steps[0].ToolResults, 3)
		assert.Equal(t, "fast", steps[0].ToolResults[0].ToolName)
		assert.Equal(t, "slow", steps[0].ToolResults[1].ToolName)
		assert.Equal(t, "medium", steps[0].ToolResults[2].ToolName)
	})

	t.Run("context_cancellation_propagates", func(t *testing.T) {
		model := &mockModel{
			streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: multiToolCallStreamParts(
					struct{ name, input string }{"blocking_a", `{}`},
					struct{ name, input string }{"blocking_b", `{}`},
				)}, nil
			},
		}

		var cancelledA, cancelledB atomic.Bool
		ctx, cancel := context.WithCancel(context.Background())

		var startedWg sync.WaitGroup
		startedWg.Add(2)

		result := StreamText(ctx, model,
			WithModelMessages(provider.UserText("go")),
			WithTools(ToolSet{
				"blocking_a": Tool{
					Description: "blocking_a",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(ctx context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						startedWg.Done()
						<-ctx.Done()
						cancelledA.Store(true)
						return nil, ctx.Err()
					},
				},
				"blocking_b": Tool{
					Description: "blocking_b",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(ctx context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						startedWg.Done()
						<-ctx.Done()
						cancelledB.Store(true)
						return nil, ctx.Err()
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)

		go func() {
			startedWg.Wait()
			cancel()
		}()

		for range result.FullStream() {
		}

		assert.True(t, cancelledA.Load(), "tool A should have received context cancellation")
		assert.True(t, cancelledB.Load(), "tool B should have received context cancellation")
	})

	t.Run("single_tool_call", func(t *testing.T) {
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: toolCallStreamParts("single", `{"x":1}`)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("result")}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("go")),
			WithTools(ToolSet{
				"single": Tool{
					Description: "single tool",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return json.RawMessage(`{"done":true}`), nil
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)

		for range result.FullStream() {
		}

		assert.Equal(t, "result", result.Text())
		steps := result.Steps()
		require.Len(t, steps, 2)
		require.Len(t, steps[0].ToolResults, 1)
		assert.Equal(t, "single", steps[0].ToolResults[0].ToolName)
		assert.Equal(t, json.RawMessage(`{"done":true}`), steps[0].ToolResults[0].Output)
	})

	t.Run("all_tools_fail", func(t *testing.T) {
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: multiToolCallStreamParts(
						struct{ name, input string }{"fail_a", `{}`},
						struct{ name, input string }{"fail_b", `{}`},
						struct{ name, input string }{"fail_c", `{}`},
					)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("recovered")}, nil
			},
		}

		var streamErrors []string
		var streamResults []string

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("go")),
			WithTools(ToolSet{
				"fail_a": Tool{
					Description: "fail_a",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return nil, fmt.Errorf("fail_a error")
					},
				},
				"fail_b": Tool{
					Description: "fail_b",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return nil, fmt.Errorf("fail_b error")
					},
				},
				"fail_c": Tool{
					Description: "fail_c",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return nil, fmt.Errorf("fail_c error")
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)

		for part := range result.FullStream() {
			switch p := part.(type) {
			case StreamToolError:
				streamErrors = append(streamErrors, p.ToolName)
			case StreamToolResult:
				streamResults = append(streamResults, p.ToolName)
			}
		}

		assert.Empty(t, streamResults, "no StreamToolResult events should be emitted when all tools fail")
		assert.Len(t, streamErrors, 3, "StreamToolError should be emitted for each failed tool")

		steps := result.Steps()
		require.GreaterOrEqual(t, len(steps), 2)
		require.Len(t, steps[0].ToolResults, 3)
		for _, tr := range steps[0].ToolResults {
			require.NotNil(t, tr.ModelOutput)
			assert.Equal(t, provider.ToolOutputErrorText, tr.ModelOutput.Type)
		}
	})

	t.Run("to_model_output_failure_independent", func(t *testing.T) {
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: multiToolCallStreamParts(
						struct{ name, input string }{"good", `{}`},
						struct{ name, input string }{"bad_convert", `{}`},
					)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("go")),
			WithTools(ToolSet{
				"good": Tool{
					Description: "good",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return json.RawMessage(`{"ok":true}`), nil
					},
				},
				"bad_convert": Tool{
					Description: "bad_convert",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return json.RawMessage(`{"raw":1}`), nil
					},
					ToModelOutput: func(_ ToolOutputContext) (*provider.ToolResultOutput, error) {
						return nil, fmt.Errorf("conversion failed")
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)

		for range result.FullStream() {
		}

		steps := result.Steps()
		require.GreaterOrEqual(t, len(steps), 2)
		require.Len(t, steps[0].ToolResults, 2)

		trGood := steps[0].ToolResults[0]
		assert.Equal(t, "good", trGood.ToolName)
		assert.Equal(t, json.RawMessage(`{"ok":true}`), trGood.Output)
		assert.Nil(t, trGood.ModelOutput)

		trBad := steps[0].ToolResults[1]
		assert.Equal(t, "bad_convert", trBad.ToolName)
		require.NotNil(t, trBad.ModelOutput)
		assert.Equal(t, provider.ToolOutputErrorText, trBad.ModelOutput.Type)
		assert.Equal(t, "conversion failed", trBad.ModelOutput.Text)
	})

	t.Run("stream_events_arrive_in_declaration_order", func(t *testing.T) {
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: multiToolCallStreamParts(
						struct{ name, input string }{"slow", `{}`},
						struct{ name, input string }{"fast", `{}`},
					)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			},
		}

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("go")),
			WithTools(ToolSet{
				"slow": Tool{
					Description: "slow",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						time.Sleep(80 * time.Millisecond)
						return json.RawMessage(`"slow"`), nil
					},
				},
				"fast": Tool{
					Description: "fast",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						time.Sleep(10 * time.Millisecond)
						return json.RawMessage(`"fast"`), nil
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
		)

		var resultOrder []string
		for part := range result.FullStream() {
			if tr, ok := part.(StreamToolResult); ok {
				resultOrder = append(resultOrder, tr.ToolName)
			}
		}

		require.Len(t, resultOrder, 2)
		assert.Equal(t, "slow", resultOrder[0], "events follow tool call declaration order, not completion order")
		assert.Equal(t, "fast", resultOrder[1], "events follow tool call declaration order, not completion order")
	})

	t.Run("callbacks_invoked_for_each_concurrent_tool", func(t *testing.T) {
		callNum := 0
		model := &mockModel{
			streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				callNum++
				if callNum == 1 {
					return &provider.StreamResult{Stream: multiToolCallStreamParts(
						struct{ name, input string }{"tool_a", `{}`},
						struct{ name, input string }{"tool_b", `{}`},
						struct{ name, input string }{"tool_c", `{}`},
					)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			},
		}

		var startCount atomic.Int32
		var finishCount atomic.Int32
		var startNames, finishNames sync.Map

		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("go")),
			WithTools(ToolSet{
				"tool_a": Tool{
					Description: "tool_a",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						time.Sleep(20 * time.Millisecond)
						return json.RawMessage(`"a"`), nil
					},
				},
				"tool_b": Tool{
					Description: "tool_b",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						time.Sleep(20 * time.Millisecond)
						return json.RawMessage(`"b"`), nil
					},
				},
				"tool_c": Tool{
					Description: "tool_c",
					InputSchema: testMustSchema(t, `{"type":"object"}`),
					Execute: func(_ context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
						return nil, fmt.Errorf("tool_c failed")
					},
				},
			}),
			WithStopWhen(StepCountIs(5)),
			OnToolCallStart(func(state OnToolCallStartState) {
				startCount.Add(1)
				startNames.Store(state.ToolCall.ToolName, true)
			}),
			OnToolCallFinish(func(state OnToolCallFinishState) {
				finishCount.Add(1)
				finishNames.Store(state.ToolCall.ToolName, true)
			}),
		)

		for range result.FullStream() {
		}

		assert.Equal(t, int32(3), startCount.Load(), "OnToolCallStart should be invoked for each tool")
		assert.Equal(t, int32(3), finishCount.Load(), "OnToolCallFinish should be invoked for each tool")

		for _, name := range []string{"tool_a", "tool_b", "tool_c"} {
			_, ok := startNames.Load(name)
			assert.True(t, ok, "OnToolCallStart should have been called for %s", name)
			_, ok = finishNames.Load(name)
			assert.True(t, ok, "OnToolCallFinish should have been called for %s", name)
		}
	})
}

func TestAppendToolResults_ProviderExecutedRouting(t *testing.T) {
	meta := provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"caller":{"type":"direct"}}`),
	}

	t.Run("provider-executed results go inline in assistant message", func(t *testing.T) {
		step := StepResult{
			ToolCalls: []ToolCall{
				{ToolCallID: "srv-1", ToolName: "web_search", Input: json.RawMessage(`{"q":"test"}`), ProviderExecuted: true},
			},
			ToolResults: []ToolResult{
				{ToolCallID: "srv-1", ToolName: "web_search", Output: json.RawMessage(`[{"url":"x"}]`), ProviderExecuted: true},
			},
		}
		msgs := appendToolResults(nil, step)
		require.Len(t, msgs, 1, "should have only assistant message, no tool message")
		am := msgs[0]
		assert.Equal(t, provider.RoleAssistant, am.Role)
		require.Len(t, am.Content, 2, "assistant should have tool-call + tool-result inline")
		assert.Equal(t, provider.ContentPartTypeToolCall, am.Content[0].Type)
		assert.Equal(t, provider.ContentPartTypeToolResult, am.Content[1].Type)
		assert.Equal(t, "srv-1", am.Content[1].ToolCallID)
	})

	t.Run("non-provider-executed results go in tool message", func(t *testing.T) {
		step := StepResult{
			ToolCalls: []ToolCall{
				{ToolCallID: "tc-1", ToolName: "my_tool", Input: json.RawMessage(`{}`)},
			},
			ToolResults: []ToolResult{
				{ToolCallID: "tc-1", ToolName: "my_tool", Output: json.RawMessage(`{"ok":true}`)},
			},
		}
		msgs := appendToolResults(nil, step)
		require.Len(t, msgs, 2)
		assert.Equal(t, provider.RoleAssistant, msgs[0].Role)
		assert.Equal(t, provider.RoleTool, msgs[1].Role)
		require.Len(t, msgs[1].Content, 1)
	})

	t.Run("mixed provider-executed and non-provider-executed", func(t *testing.T) {
		step := StepResult{
			ToolCalls: []ToolCall{
				{ToolCallID: "srv-1", ToolName: "web_search", Input: json.RawMessage(`{}`), ProviderExecuted: true},
				{ToolCallID: "tc-1", ToolName: "report", Input: json.RawMessage(`{}`)},
			},
			ToolResults: []ToolResult{
				{ToolCallID: "srv-1", ToolName: "web_search", Output: json.RawMessage(`[]`), ProviderExecuted: true},
				{ToolCallID: "tc-1", ToolName: "report", Output: json.RawMessage(`{"ok":true}`)},
			},
		}
		msgs := appendToolResults(nil, step)
		require.Len(t, msgs, 2, "assistant + tool message")

		am := msgs[0]
		require.Len(t, am.Content, 3, "2 tool calls + 1 inline result")
		assert.Equal(t, provider.ContentPartTypeToolResult, am.Content[1].Type, "provider-executed result should be inline after its call")
		assert.Equal(t, provider.ContentPartTypeToolCall, am.Content[2].Type)
		assert.Equal(t, "tc-1", am.Content[2].ToolCallID)

		tm := msgs[1]
		require.Len(t, tm.Content, 1, "only non-provider-executed result")
		assert.Equal(t, "tc-1", tm.Content[0].ToolCallID)
	})

	t.Run("ProviderMetadata carried through", func(t *testing.T) {
		step := StepResult{
			ToolCalls: []ToolCall{
				{ToolCallID: "tc-1", ToolName: "t", Input: json.RawMessage(`{}`), ProviderMetadata: meta},
			},
			ToolResults: []ToolResult{
				{ToolCallID: "tc-1", ToolName: "t", Output: json.RawMessage(`{}`), ProviderExecuted: true, ProviderMetadata: meta},
			},
		}
		msgs := appendToolResults(nil, step)
		am := msgs[0]
		assert.NotNil(t, am.Content[0].ProviderOptions, "ToolCall ProviderOptions should be set")
		assert.NotNil(t, am.Content[1].ProviderOptions, "ToolResult ProviderOptions should be set")
	})

	t.Run("ModelOutput preserved for provider-executed results", func(t *testing.T) {
		modelOutput := &provider.ToolResultOutput{
			Type: provider.ToolOutputText,
			Text: "custom output",
		}
		step := StepResult{
			ToolCalls: []ToolCall{
				{ToolCallID: "srv-1", ToolName: "ws", Input: json.RawMessage(`{}`), ProviderExecuted: true},
			},
			ToolResults: []ToolResult{
				{ToolCallID: "srv-1", ToolName: "ws", Output: json.RawMessage(`{}`), ModelOutput: modelOutput, ProviderExecuted: true},
			},
		}
		msgs := appendToolResults(nil, step)
		am := msgs[0]
		require.NotNil(t, am.Content[1].Output)
		assert.Equal(t, provider.ToolOutputText, am.Content[1].Output.Type)
		assert.Equal(t, "custom output", am.Content[1].Output.Text)
	})

	t.Run("reasoning preserved across tool-result rounds (#171)", func(t *testing.T) {
		sigMeta := provider.ProviderMetadata{
			"anthropic": json.RawMessage(`{"signature":"sig_xyz"}`),
		}
		step := StepResult{
			Reasoning: []ReasoningOutput{
				ReasoningTextOutput{Text: "thinking step", ProviderMetadata: sigMeta},
			},
			Text: "after thinking",
			ToolCalls: []ToolCall{
				{ToolCallID: "tc-1", ToolName: "weather", Input: json.RawMessage(`{"city":"Tokyo"}`)},
			},
			ToolResults: []ToolResult{
				{ToolCallID: "tc-1", ToolName: "weather", Output: json.RawMessage(`"72F sunny"`)},
			},
		}
		msgs := appendToolResults(nil, step)
		require.Len(t, msgs, 2, "assistant + tool message")

		am := msgs[0]
		require.Len(t, am.Content, 3, "reasoning + text + tool-call in assistant message")
		assert.Equal(t, provider.ContentPartTypeReasoning, am.Content[0].Type, "reasoning must come first")
		assert.Equal(t, "thinking step", am.Content[0].Text)

		require.NotNil(t, am.Content[0].ProviderOptions, "reasoning ProviderOptions must carry the signature")
		anthOpt := am.Content[0].ProviderOptions["anthropic"]
		require.NotNil(t, anthOpt)
		raw, ok := anthOpt.(provider.RawProviderOption)
		require.True(t, ok, "expected RawProviderOption, got %T", anthOpt)
		assert.JSONEq(t, `{"signature":"sig_xyz"}`, string(raw.Raw))

		assert.Equal(t, provider.ContentPartTypeText, am.Content[1].Type)
		assert.Equal(t, provider.ContentPartTypeToolCall, am.Content[2].Type)
	})

	t.Run("text omitted when empty", func(t *testing.T) {
		step := StepResult{
			ToolCalls: []ToolCall{
				{ToolCallID: "tc-1", ToolName: "t", Input: json.RawMessage(`{}`)},
			},
		}
		msgs := appendToolResults(nil, step)
		require.Len(t, msgs, 1, "no tool results yet, only assistant message")
		am := msgs[0]
		require.Len(t, am.Content, 1, "only the tool-call part")
		assert.Equal(t, provider.ContentPartTypeToolCall, am.Content[0].Type)
	})

	t.Run("approval requests are retained in response messages", func(t *testing.T) {
		step := StepResult{
			ToolCalls: []ToolCall{
				{ToolCallID: "tc-1", ToolName: "my_tool", Input: json.RawMessage(`{}`)},
			},
			ToolApprovalRequests: []ToolApprovalRequest{
				{ApprovalID: "apr_1", ToolCallID: "tc-1", ToolName: "my_tool"},
			},
			ToolApprovalResponses: []ToolApprovalResponse{
				{ApprovalID: "apr_1", ToolCallID: "tc-1", ToolName: "my_tool", Approved: true},
			},
			ToolResults: []ToolResult{
				{ToolCallID: "tc-1", ToolName: "my_tool", Output: json.RawMessage(`{"ok":true}`)},
			},
		}
		msgs := appendToolResults(nil, step)
		require.Len(t, msgs, 2)
		require.Len(t, msgs[0].Content, 2)
		assert.Equal(t, provider.ContentPartTypeToolCall, msgs[0].Content[0].Type)
		assert.Equal(t, provider.ContentPartTypeToolApprovalRequest, msgs[0].Content[1].Type)
		require.Len(t, msgs[1].Content, 2)
		assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, msgs[1].Content[0].Type)
		assert.Equal(t, provider.ContentPartTypeToolResult, msgs[1].Content[1].Type)
	})
}

func TestStreamText_UnresolvedToolCalls(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: textStreamParts("should not run")}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(
			provider.UserText("run tools"),
			provider.NewAssistantMessage(
				provider.ToolCallPart("call-first", "first", json.RawMessage(`{}`)),
				provider.ContentPart{
					Type:             provider.ContentPartTypeToolCall,
					ToolCallID:       "call-provider",
					ToolName:         "provider_tool",
					Input:            json.RawMessage(`{}`),
					ProviderExecuted: true,
				},
				provider.ToolCallPart("call-second", "second", json.RawMessage(`{}`)),
			),
		),
	)
	for range result.FullStream() {
	}

	var missingErr *MissingToolResultsError
	require.ErrorAs(t, result.Err(), &missingErr)
	assert.Equal(t, []string{"call-first", "call-second"}, missingErr.ToolCallIDs)
	assert.Equal(t, "aisdk: tool results are missing for tool calls call-first, call-second", missingErr.Error())
	assert.Equal(t, 0, model.callCount)
}

func TestSanitizePromptForProvider_ToolApprovalBookkeeping(t *testing.T) {
	msgs := []provider.Message{
		provider.UserText("hi"),
		provider.NewAssistantMessage(
			provider.ToolCallPart("c1", "local", json.RawMessage(`{}`)),
			provider.ToolApprovalRequestPart("apr_1", "c1", false),
		),
		provider.NewToolMessage(provider.ToolApprovalResponsePart("apr_1", true, "ok")),
		provider.NewToolMessage(provider.ProviderExecutedToolApprovalResponsePart("apr_2", false, "blocked")),
	}

	sanitized, err := sanitizePromptForProvider(msgs)
	require.NoError(t, err)
	require.Len(t, sanitized, 3)
	require.Equal(t, provider.RoleAssistant, sanitized[1].Role)
	require.Len(t, sanitized[1].Content, 1)
	assert.Equal(t, provider.ContentPartTypeToolCall, sanitized[1].Content[0].Type)
	require.Equal(t, provider.RoleTool, sanitized[2].Role)
	require.Len(t, sanitized[2].Content, 1)
	assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, sanitized[2].Content[0].Type)
	assert.True(t, sanitized[2].Content[0].ProviderExecuted)
}

func TestStreamTextContent_TextDeltaMetadata(t *testing.T) {
	startMetadata := provider.ProviderMetadata{"test": json.RawMessage(`{"stage":"start"}`)}
	deltaMetadata := provider.ProviderMetadata{"test": json.RawMessage(`{"stage":"delta"}`)}
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 5)
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1", ProviderMetadata: startMetadata}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "answer", ProviderMetadata: deltaMetadata}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}

	result := StreamText(context.Background(), model, WithModelMessages(provider.UserText("test")))
	for range result.FullStream() {
	}

	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].Content, 1)
	text, ok := steps[0].Content[0].(TextContent)
	require.True(t, ok)
	assert.Equal(t, deltaMetadata, text.ProviderMetadata)
	require.Len(t, steps[0].Response.Messages, 1)
	require.Len(t, steps[0].Response.Messages[0].Content, 1)
	assert.Equal(t, providerMetadataToOptions(deltaMetadata), steps[0].Response.Messages[0].Content[0].ProviderOptions)
}

func TestStreamTextContent_CustomPartPreserved(t *testing.T) {
	metadata := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"cmp-1"}`)}
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 2)
		stream <- provider.StreamPart{Type: provider.PartCustom, Kind: "openai.compaction", ProviderMetadata: metadata}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}

	result := StreamText(context.Background(), model, WithModelMessages(provider.UserText("test")))
	var streamed []StreamCustom
	for part := range result.FullStream() {
		if custom, ok := part.(StreamCustom); ok {
			streamed = append(streamed, custom)
		}
	}

	require.Len(t, streamed, 1)
	assert.Equal(t, "openai.compaction", streamed[0].Kind)
	assert.Equal(t, metadata, streamed[0].ProviderMetadata)
	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].Content, 1)
	custom, ok := steps[0].Content[0].(CustomContent)
	require.True(t, ok)
	assert.Equal(t, "openai.compaction", custom.Kind)
	assert.Equal(t, metadata, custom.ProviderMetadata)
	require.Len(t, steps[0].Response.Messages, 1)
	require.Len(t, steps[0].Response.Messages[0].Content, 1)
	assert.Equal(t, provider.ContentPartTypeCustom, steps[0].Response.Messages[0].Content[0].Type)
}

func TestStreamTextContent_EmptyRecordedTextPreserved(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 5)
		stream <- provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte("before")}, MediaType: "text/plain"}
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte("after")}, MediaType: "text/plain"}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}

	result := StreamText(context.Background(), model, WithModelMessages(provider.UserText("test")))
	for range result.FullStream() {
	}

	require.NoError(t, result.Err())
	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].Content, 3)
	assert.IsType(t, FileContent{}, steps[0].Content[0])
	text, ok := steps[0].Content[1].(TextContent)
	require.True(t, ok)
	assert.Empty(t, text.Text)
	assert.IsType(t, FileContent{}, steps[0].Content[2])

	require.Len(t, steps[0].Response.Messages, 1)
	responseContent := steps[0].Response.Messages[0].Content
	require.Len(t, responseContent, 2)
	assert.Equal(t, provider.ContentPartTypeFile, responseContent[0].Type)
	assert.Equal(t, provider.ContentPartTypeFile, responseContent[1].Type)
}

func TestStreamTextContent_LocalApprovalFollowsRecordedContent(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 5)
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "dangerous", Input: `{}`}
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "after call"}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}

	result := StreamText(context.Background(), model,
		WithModelMessages(provider.UserText("test")),
		WithGenerateID(func() string { return "approval-1" }),
		WithTools(ToolSet{"dangerous": {
			InputSchema:   testMustSchema(t, `{"type":"object"}`),
			NeedsApproval: ApprovalRequired(),
			Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				return json.RawMessage(`{"ok":true}`), nil
			},
		}}),
	)
	for range result.FullStream() {
	}

	require.NoError(t, result.Err())
	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].Content, 3)
	assert.IsType(t, ToolCallContent{}, steps[0].Content[0])
	assert.IsType(t, TextContent{}, steps[0].Content[1])
	assert.IsType(t, ToolApprovalRequestContent{}, steps[0].Content[2])

	require.Len(t, steps[0].Response.Messages, 1)
	responseContent := steps[0].Response.Messages[0].Content
	require.Len(t, responseContent, 3)
	assert.Equal(t, provider.ContentPartTypeToolCall, responseContent[0].Type)
	assert.Equal(t, provider.ContentPartTypeText, responseContent[1].Type)
	assert.Equal(t, provider.ContentPartTypeToolApprovalRequest, responseContent[2].Type)
}

func TestStreamTextContent_PreliminaryProviderResultExcluded(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 5)
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "provider-tool", Input: `{}`, ProviderExecuted: true}
		stream <- provider.StreamPart{
			Type: provider.PartToolResult, ToolCallID: "call-1", ToolName: "provider-tool", ProviderExecuted: true, Preliminary: boolPtr(true),
			Result: json.RawMessage(`{"stage":"preliminary"}`),
		}
		stream <- provider.StreamPart{
			Type: provider.PartToolResult, ToolCallID: "call-1", ToolName: "provider-tool", ProviderExecuted: true,
			Result: json.RawMessage(`{"stage":"final"}`),
		}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}

	result := StreamText(context.Background(), model, WithModelMessages(provider.UserText("test")))
	for range result.FullStream() {
	}

	steps := result.Steps()
	require.Len(t, steps, 1)
	require.Len(t, steps[0].ToolResults, 1)
	require.Len(t, steps[0].Content, 2)
	assert.IsType(t, ToolCallContent{}, steps[0].Content[0])
	toolResult, ok := steps[0].Content[1].(ToolResultContent)
	require.True(t, ok)
	assert.False(t, toolResult.Preliminary)
	assert.JSONEq(t, `{"stage":"final"}`, string(toolResult.Output))
	require.Len(t, steps[0].Response.Messages, 1)
	require.Len(t, steps[0].Response.Messages[0].Content, 2)
	assert.Equal(t, provider.ContentPartTypeToolResult, steps[0].Response.Messages[0].Content[1].Type)
	require.NotNil(t, steps[0].Response.Messages[0].Content[1].Output)
	assert.JSONEq(t, `{"stage":"final"}`, string(steps[0].Response.Messages[0].Content[1].Output.JSON))
}

func TestBuildContent(t *testing.T) {
	t.Run("recorded content preserves provider order and metadata", func(t *testing.T) {
		fileMetadata := provider.ProviderMetadata{"test": json.RawMessage(`{"kind":"file"}`)}
		textMetadata := provider.ProviderMetadata{"test": json.RawMessage(`{"kind":"text"}`)}
		reasoningMetadata := provider.ProviderMetadata{"test": json.RawMessage(`{"kind":"reasoning"}`)}
		sourceMetadata := provider.ProviderMetadata{"test": json.RawMessage(`{"kind":"source"}`)}
		toolMetadata := provider.ProviderMetadata{"test": json.RawMessage(`{"kind":"tool"}`)}
		dynamic := true
		step := StepResult{
			responseContent: []provider.ContentPart{
				{Type: provider.ContentPartTypeFile, Data: &provider.DataContent{Bytes: []byte{1, 2, 3}}, MediaType: "image/png", ProviderOptions: providerMetadataToOptions(fileMetadata)},
				{Type: provider.ContentPartTypeToolCall, ToolCallID: "call-1"},
				{Type: provider.ContentPartTypeText, Text: "answer", ProviderOptions: providerMetadataToOptions(textMetadata)},
				{Type: provider.ContentPartTypeReasoning, Text: "thinking", ProviderOptions: providerMetadataToOptions(reasoningMetadata)},
				{Type: provider.ContentPartTypeSource, SourceType: provider.SourceTypeURL, ID: "source-1", URL: "https://example.com", Title: "Example", ProviderOptions: providerMetadataToOptions(sourceMetadata)},
				{Type: provider.ContentPartTypeToolResult, ToolCallID: "call-1"},
				{Type: provider.ContentPartTypeReasoningFile, Data: &provider.DataContent{Base64: "AQID"}, MediaType: "image/png", ProviderOptions: providerMetadataToOptions(reasoningMetadata)},
			},
			ToolCalls: []ToolCall{{
				ToolCallID: "call-1", ToolName: "search", Input: json.RawMessage(`{"query":"test"}`),
				ProviderExecuted: true, Dynamic: &dynamic, Title: "Search", ProviderMetadata: toolMetadata,
			}},
			ToolResults: []ToolResult{{
				ToolCallID: "call-1", ToolName: "search", Input: json.RawMessage(`{"query":"test"}`), Output: json.RawMessage(`{"ok":true}`),
				ProviderExecuted: true, Dynamic: &dynamic, Title: "Search", ProviderMetadata: toolMetadata,
			}},
		}

		content := buildContent(step)
		require.Len(t, content, 7)

		file, ok := content[0].(FileContent)
		require.True(t, ok)
		assert.Equal(t, []byte{1, 2, 3}, file.File.Data)
		assert.Equal(t, fileMetadata, file.ProviderMetadata)
		call, ok := content[1].(ToolCallContent)
		require.True(t, ok)
		assert.Equal(t, "Search", call.Title)
		assert.Equal(t, toolMetadata, call.ProviderMetadata)
		text, ok := content[2].(TextContent)
		require.True(t, ok)
		assert.Equal(t, "answer", text.Text)
		assert.Equal(t, textMetadata, text.ProviderMetadata)
		reasoning, ok := content[3].(ReasoningContent)
		require.True(t, ok)
		assert.Equal(t, reasoningMetadata, reasoning.ProviderMetadata)
		source, ok := content[4].(SourceContent)
		require.True(t, ok)
		assert.Equal(t, "source-1", source.Source.ID)
		assert.Equal(t, sourceMetadata, source.Source.ProviderMetadata)
		result, ok := content[5].(ToolResultContent)
		require.True(t, ok)
		assert.Equal(t, json.RawMessage(`{"ok":true}`), result.Output)
		assert.Equal(t, toolMetadata, result.ProviderMetadata)
		reasoningFile, ok := content[6].(ReasoningFileContent)
		require.True(t, ok)
		assert.Equal(t, "AQID", reasoningFile.File.Base64)
		assert.Equal(t, reasoningMetadata, reasoningFile.ProviderMetadata)
	})

	t.Run("recorded content appends local parts exactly once", func(t *testing.T) {
		step := StepResult{
			responseContent: []provider.ContentPart{
				{Type: provider.ContentPartTypeToolCall, ToolCallID: "provider-call"},
				{Type: provider.ContentPartTypeToolResult, ToolCallID: "provider-call"},
				{Type: provider.ContentPartTypeToolCall, ToolCallID: "local-call"},
			},
			ToolCalls: []ToolCall{
				{ToolCallID: "provider-call", ToolName: "provider"},
				{ToolCallID: "local-call", ToolName: "local"},
			},
			ToolApprovalRequests: []ToolApprovalRequest{
				{ApprovalID: "approval-1", ToolCallID: "local-call", ToolName: "local"},
			},
			ToolApprovalResponses: []ToolApprovalResponse{
				{ApprovalID: "approval-1", ToolCallID: "local-call", ToolName: "local", Approved: true},
			},
			ToolResults: []ToolResult{
				{ToolCallID: "provider-call", ToolName: "provider", Output: json.RawMessage(`{"stage":"preliminary"}`), ProviderExecuted: true, Preliminary: true},
				{ToolCallID: "provider-call", ToolName: "provider", Output: json.RawMessage(`{"stage":"final"}`), ProviderExecuted: true},
				{ToolCallID: "local-call", ToolName: "local", Output: json.RawMessage(`{"ok":true}`)},
			},
		}

		content := buildContent(step)
		require.Len(t, content, 6)
		require.IsType(t, ToolCallContent{}, content[0])
		require.IsType(t, ToolResultContent{}, content[1])
		require.IsType(t, ToolCallContent{}, content[2])
		require.IsType(t, ToolApprovalRequestContent{}, content[3])
		require.IsType(t, ToolApprovalResponseContent{}, content[4])
		require.IsType(t, ToolResultContent{}, content[5])
		assert.Equal(t, "provider-call", content[0].(ToolCallContent).ToolCallID)
		assert.Equal(t, "provider-call", content[1].(ToolResultContent).ToolCallID)
		assert.JSONEq(t, `{"stage":"final"}`, string(content[1].(ToolResultContent).Output))
		assert.Equal(t, "local-call", content[2].(ToolCallContent).ToolCallID)
		assert.Equal(t, "approval-1", content[3].(ToolApprovalRequestContent).ApprovalID)
		assert.Equal(t, "approval-1", content[4].(ToolApprovalResponseContent).ApprovalID)
		assert.Equal(t, "local-call", content[5].(ToolResultContent).ToolCallID)
	})

	t.Run("grouped fallback keeps approval parts with tool call", func(t *testing.T) {
		step := StepResult{
			ToolCalls: []ToolCall{
				{ToolCallID: "c1", ToolName: "dangerous", Input: json.RawMessage(`{}`)},
				{ToolCallID: "c2", ToolName: "safe", Input: json.RawMessage(`{}`)},
			},
			ToolApprovalRequests: []ToolApprovalRequest{
				{ApprovalID: "apr_1", ToolCallID: "c1", ToolName: "dangerous"},
			},
			ToolApprovalResponses: []ToolApprovalResponse{
				{ApprovalID: "apr_1", ToolCallID: "c1", ToolName: "dangerous", Approved: false, Reason: "unsafe"},
			},
			ToolResults: []ToolResult{
				{ToolCallID: "c1", ToolName: "dangerous", ModelOutput: &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: "unsafe"}},
				{ToolCallID: "c2", ToolName: "safe", Output: json.RawMessage(`{"ok":true}`)},
			},
		}

		content := buildContent(step)
		require.Len(t, content, 6)
		assert.IsType(t, ToolCallContent{}, content[0])
		assert.IsType(t, ToolApprovalRequestContent{}, content[1])
		assert.IsType(t, ToolApprovalResponseContent{}, content[2])
		assert.IsType(t, ToolResultContent{}, content[3])
		assert.IsType(t, ToolCallContent{}, content[4])
		assert.IsType(t, ToolResultContent{}, content[5])
	})
}

func TestIsDynamic_UnknownToolPreservesProviderValue(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		assert.Equal(t, boolPtr(false), isDynamic("provider.tool", nil, nil))
	})
	t.Run("explicit false", func(t *testing.T) {
		assert.Equal(t, boolPtr(false), isDynamic("provider.tool", boolPtr(false), nil))
	})
	t.Run("explicit true", func(t *testing.T) {
		assert.Equal(t, boolPtr(true), isDynamic("provider.tool", boolPtr(true), nil))
	})
}

func boolPtr(b bool) *bool { return &b }
