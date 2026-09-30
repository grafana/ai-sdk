package aisdk_test

import (
	"context"
	"encoding/json"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generatedOutputStream(text string, reason provider.UnifiedFinishReason) <-chan provider.StreamPart {
	parts := make(chan provider.StreamPart, 4)
	if text != "" {
		parts <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: text}
		parts <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
	}
	parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: reason}}
	close(parts)
	return parts
}

func TestGeneratedOutput_FinalFinishReason(t *testing.T) {
	type recipe struct {
		Name string `json:"name"`
	}
	type city struct {
		Name string `json:"name"`
	}

	objectOutput, err := output.Object[recipe](mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`))
	require.NoError(t, err)
	arrayOutput, err := output.Array[city](mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`))
	require.NoError(t, err)
	choiceOutput, err := output.Choice("sunny", "rainy")
	require.NoError(t, err)

	modes := []struct {
		name       string
		output     aisdk.Output
		valid      string
		invalid    string
		want       any
		zero       any
		typedValue func(*aisdk.GenerateTextResult) (any, error)
	}{
		{
			name: "object", output: objectOutput, valid: `{"name":"Ada"}`, invalid: `{"name":42}`, want: recipe{Name: "Ada"}, zero: recipe{},
			typedValue: func(result *aisdk.GenerateTextResult) (any, error) {
				return output.Value[recipe](&output.ObjectResult[recipe]{GenerateTextResult: result})
			},
		},
		{
			name: "array", output: arrayOutput, valid: `{"elements":[{"name":"Paris"}]}`, invalid: `{"elements":[{"name":42}]}`, want: []city{{Name: "Paris"}}, zero: []city(nil),
			typedValue: func(result *aisdk.GenerateTextResult) (any, error) {
				return output.Value[[]city](&output.ObjectResult[[]city]{GenerateTextResult: result})
			},
		},
		{
			name: "choice", output: choiceOutput, valid: `{"result":"sunny"}`, invalid: `{"result":"cloudy"}`, want: "sunny", zero: "",
			typedValue: func(result *aisdk.GenerateTextResult) (any, error) {
				return output.Value[string](&output.ObjectResult[string]{GenerateTextResult: result})
			},
		},
		{
			name: "json", output: output.JSON(), valid: `{"name":"Ada"}`, invalid: `{broken`, want: map[string]any{"name": "Ada"}, zero: map[string]any(nil),
			typedValue: func(result *aisdk.GenerateTextResult) (any, error) {
				return output.Value[map[string]any](&output.ObjectResult[map[string]any]{GenerateTextResult: result})
			},
		},
	}
	reasons := []provider.UnifiedFinishReason{
		provider.FinishReasonStop,
		provider.FinishReasonLength,
		provider.FinishReasonContentFilter,
		provider.FinishReasonError,
		provider.FinishReasonOther,
		provider.FinishReasonToolCalls,
	}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, reason := range reasons {
				t.Run(string(reason), func(t *testing.T) {
					for _, input := range []struct {
						name string
						text string
					}{
						{"valid", mode.valid},
						{"invalid", mode.invalid},
						{"empty", ""},
					} {
						t.Run(input.name, func(t *testing.T) {
							for _, entry := range []string{"GenerateText", "Agent.Generate"} {
								t.Run(entry, func(t *testing.T) {
									model := &testModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
										return &provider.StreamResult{Stream: generatedOutputStream(input.text, reason)}, nil
									}}
									var result *aisdk.GenerateTextResult
									var err error
									if entry == "GenerateText" {
										result, err = aisdk.GenerateText(context.Background(), model, aisdk.WithModelMessages(provider.UserText("output")), aisdk.WithOutput(mode.output))
									} else {
										agent := aisdk.NewToolLoopAgent(model, aisdk.WithToolLoopAgentOptions(aisdk.WithOutput(mode.output)))
										result, err = agent.Generate(context.Background(), aisdk.WithAgentPrompt("output"))
									}
									require.NoError(t, err)
									assert.Equal(t, input.text, result.Text)
									assert.Equal(t, reason, result.FinishReason.Unified)

									value, valueErr := mode.typedValue(result)
									switch {
									case reason == provider.FinishReasonToolCalls || (reason != provider.FinishReasonStop && input.text == ""):
										assert.Nil(t, result.Output)
										assert.NoError(t, result.OutputError)
										assert.Equal(t, mode.zero, value)
										assert.ErrorIs(t, valueErr, aisdk.ErrNoObjectGenerated)
									case input.name == "valid":
										assert.Equal(t, mode.want, result.Output)
										assert.NoError(t, result.OutputError)
										assert.Equal(t, mode.want, value)
										assert.NoError(t, valueErr)
									default:
										assert.Nil(t, result.Output)
										assert.ErrorIs(t, result.OutputError, aisdk.ErrNoObjectGenerated)
										assert.Equal(t, mode.zero, value)
										assert.ErrorIs(t, valueErr, aisdk.ErrNoObjectGenerated)
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

func TestGeneratedOutput_JSONNull(t *testing.T) {
	for _, entry := range []string{"GenerateText", "Agent.Generate"} {
		t.Run(entry, func(t *testing.T) {
			model := &testModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: generatedOutputStream("null", provider.FinishReasonStop)}, nil
			}}
			var result *aisdk.GenerateTextResult
			var err error
			if entry == "GenerateText" {
				result, err = aisdk.GenerateText(context.Background(), model, aisdk.WithModelMessages(provider.UserText("output")), aisdk.WithOutput(output.JSON()))
			} else {
				agent := aisdk.NewToolLoopAgent(model, aisdk.WithToolLoopAgentOptions(aisdk.WithOutput(output.JSON())))
				result, err = agent.Generate(context.Background(), aisdk.WithAgentPrompt("output"))
			}
			require.NoError(t, err)
			assert.Nil(t, result.Output)
			assert.NoError(t, result.OutputError)
			_, valueErr := output.Value[any](&output.ObjectResult[any]{GenerateTextResult: result})
			assert.ErrorIs(t, valueErr, aisdk.ErrNoObjectGenerated)
		})
	}
}

func TestGeneratedOutput_UsesFinalStepAfterTool(t *testing.T) {
	type recipe struct {
		Name string `json:"name"`
	}
	out, err := output.Object[recipe](mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`))
	require.NoError(t, err)

	for _, entry := range []string{"GenerateText", "Agent.Generate"} {
		t.Run(entry, func(t *testing.T) {
			calls := 0
			model := &testModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				if calls == 1 {
					parts := make(chan provider.StreamPart, 5)
					parts <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
					parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `{invalid`}
					parts <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
					parts <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "lookup", Input: `{}`}
					parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				}
				return &provider.StreamResult{Stream: generatedOutputStream(`{"name":"Final"}`, provider.FinishReasonLength)}, nil
			}}
			tools := aisdk.ToolSet{"lookup": {
				InputSchema: mustSchema(t, `{"type":"object"}`),
				Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{"ok":true}`), nil
				},
			}}
			var result *aisdk.GenerateTextResult
			var generateErr error
			if entry == "GenerateText" {
				result, generateErr = aisdk.GenerateText(context.Background(), model, aisdk.WithModelMessages(provider.UserText("output")), aisdk.WithOutput(out), aisdk.WithTools(tools), aisdk.WithStopWhen(aisdk.StepCountIs(3)))
			} else {
				agent := aisdk.NewToolLoopAgent(model, aisdk.WithToolLoopAgentOptions(aisdk.WithOutput(out), aisdk.WithTools(tools), aisdk.WithStopWhen(aisdk.StepCountIs(3))))
				result, generateErr = agent.Generate(context.Background(), aisdk.WithAgentPrompt("output"))
			}
			require.NoError(t, generateErr)
			assert.Equal(t, 2, calls)
			assert.Len(t, result.Steps, 2)
			assert.Equal(t, `{"name":"Final"}`, result.Text)
			assert.Equal(t, provider.FinishReasonLength, result.FinishReason.Unified)
			assert.NoError(t, result.OutputError)
			assert.Equal(t, recipe{Name: "Final"}, result.Output)
		})
	}
}
