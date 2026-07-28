package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type weatherInput struct {
	City string `json:"city"`
}

type weatherOutput struct {
	Temp float64 `json:"temp"`
	Unit string  `json:"unit"`
}

func TestTypedTool(t *testing.T) {
	t.Run("MinimalDefinition", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "Get current weather",
			Execute: func(_ context.Context, input weatherInput, _ ToolExecutionOptions) (weatherOutput, error) {
				return weatherOutput{Temp: 72.0, Unit: "F"}, nil
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "Get current weather", tool.Description)
		assert.NotNil(t, tool.Execute)
		assert.NotNil(t, tool.InputSchema.JSON())
	})

	t.Run("FullDefinition", func(t *testing.T) {
		outputSchema, err := schema.SchemaFor[weatherOutput]()
		require.NoError(t, err)

		toModelOutput := func(_ string, _ weatherInput, _ weatherOutput) (*provider.ToolResultOutput, error) {
			return nil, nil
		}

		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Name:        "weather",
			Description: "Get current weather",
			Title:       "Weather Tool",
			Execute: func(_ context.Context, input weatherInput, _ ToolExecutionOptions) (weatherOutput, error) {
				return weatherOutput{}, nil
			},
			OutputSchema:    outputSchema,
			InputExamples:   []weatherInput{{City: "London"}, {City: "Paris"}},
			Strict:          boolPtr(false),
			ProviderOptions: map[string]provider.ProviderOption{"anthropic": provider.RawProviderOption{Key: "anthropic"}},
			ValidateInput: func(input weatherInput) error {
				if input.City == "" {
					return errors.New("city required")
				}
				return nil
			},
			ToModelOutput: toModelOutput,
		})
		require.NoError(t, err)

		assert.Equal(t, "Get current weather", tool.Description)
		assert.Equal(t, "Weather Tool", tool.Title)
		assert.Equal(t, boolPtr(false), tool.Strict)
		assert.NotNil(t, tool.ProviderOptions)
		assert.NotNil(t, tool.ToModelOutput)
		assert.NotNil(t, tool.ValidateInput)
		assert.Equal(t, outputSchema.JSON(), tool.OutputSchema.JSON())
		require.Len(t, tool.InputExamples, 2)
	})

	t.Run("SchemaDerivation", func(t *testing.T) {
		type input struct {
			Name  string  `json:"name"`
			Count int     `json:"count"`
			Score float64 `json:"score"`
			Tags  *string `json:"tags,omitempty"`
		}

		tool, err := TypedTool(TypedToolDef[input, string]{
			Description: "test",
			Execute: func(_ context.Context, in input, _ ToolExecutionOptions) (string, error) {
				return "", nil
			},
		})
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(tool.InputSchema.JSON(), &m))

		assert.Equal(t, "object", m["type"])

		props, ok := m["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "name")
		assert.Contains(t, props, "count")
		assert.Contains(t, props, "score")
		assert.Contains(t, props, "tags")

		required, ok := m["required"].([]any)
		require.True(t, ok)
		assert.Contains(t, required, "name")
		assert.Contains(t, required, "count")
		assert.Contains(t, required, "score")
		assert.NotContains(t, required, "tags")
	})

	t.Run("NilExecute", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "no-execute tool",
		})
		require.NoError(t, err)
		assert.Nil(t, tool.Execute)
		assert.NotNil(t, tool.InputSchema.JSON())
	})

	t.Run("SchemaDerivationFailure", func(t *testing.T) {
		type unsupported struct {
			Ch chan int `json:"ch"`
		}

		_, err := TypedTool(TypedToolDef[unsupported, string]{
			Description: "should fail",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aisdk: deriving input schema")
	})

	t.Run("CallbacksPassthrough", func(t *testing.T) {
		var startCalled, deltaCalled bool
		var receivedInput weatherInput
		var receivedErr error

		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "with callbacks",
			OnInputStart: func(_ ToolExecutionOptions) {
				startCalled = true
			},
			OnInputDelta: func(_ string, _ ToolExecutionOptions) {
				deltaCalled = true
			},
			OnInputAvailable: func(input weatherInput, err error, _ ToolExecutionOptions) {
				receivedInput = input
				receivedErr = err
			},
		})
		require.NoError(t, err)

		require.NotNil(t, tool.OnInputStart)
		require.NotNil(t, tool.OnInputDelta)
		require.NotNil(t, tool.OnInputAvailable)

		tool.OnInputStart(ToolExecutionOptions{})
		tool.OnInputDelta("delta", ToolExecutionOptions{})
		tool.OnInputAvailable(json.RawMessage(`{"city":"Berlin"}`), ToolExecutionOptions{})

		assert.True(t, startCalled)
		assert.True(t, deltaCalled)
		assert.NoError(t, receivedErr)
		assert.Equal(t, "Berlin", receivedInput.City)
	})

	t.Run("OnInputAvailableUnmarshalFailure", func(t *testing.T) {
		var receivedErr error

		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "with callbacks",
			OnInputAvailable: func(_ weatherInput, err error, _ ToolExecutionOptions) {
				receivedErr = err
			},
		})
		require.NoError(t, err)

		tool.OnInputAvailable(json.RawMessage(`not-valid-json`), ToolExecutionOptions{})
		require.Error(t, receivedErr)
		assert.Contains(t, receivedErr.Error(), "aisdk: unmarshaling tool input")
	})

	t.Run("NilOnInputAvailable", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "no callbacks",
		})
		require.NoError(t, err)
		assert.Nil(t, tool.OnInputAvailable)
	})
}

func TestTypedTool_ToModelOutput(t *testing.T) {
	t.Run("ReceivesTypedInputAndOutput", func(t *testing.T) {
		var receivedID string
		var receivedInput weatherInput
		var receivedOutput weatherOutput

		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
			ToModelOutput: func(toolCallID string, input weatherInput, output weatherOutput) (*provider.ToolResultOutput, error) {
				receivedID = toolCallID
				receivedInput = input
				receivedOutput = output
				return &provider.ToolResultOutput{
					Type: provider.ToolOutputText,
					Text: output.Unit + ": " + input.City,
				}, nil
			},
		})
		require.NoError(t, err)
		require.NotNil(t, tool.ToModelOutput)

		result, err := tool.ToModelOutput(ToolOutputContext{
			ToolCallID: "call-1",
			Input:      json.RawMessage(`{"city":"Berlin"}`),
			Output:     json.RawMessage(`{"temp":20.5,"unit":"C"}`),
		})
		require.NoError(t, err)

		assert.Equal(t, "call-1", receivedID)
		assert.Equal(t, "Berlin", receivedInput.City)
		assert.Equal(t, 20.5, receivedOutput.Temp)
		assert.Equal(t, "C", receivedOutput.Unit)
		assert.Equal(t, provider.ToolOutputText, result.Type)
		assert.Equal(t, "C: Berlin", result.Text)
	})

	t.Run("InputUnmarshalFailure", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
			ToModelOutput: func(_ string, _ weatherInput, _ weatherOutput) (*provider.ToolResultOutput, error) {
				return nil, nil
			},
		})
		require.NoError(t, err)

		_, err = tool.ToModelOutput(ToolOutputContext{
			ToolCallID: "call-1",
			Input:      json.RawMessage(`bad-json`),
			Output:     json.RawMessage(`{"temp":20.5,"unit":"C"}`),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aisdk: unmarshaling tool input for ToModelOutput")
	})

	t.Run("OutputUnmarshalFailure", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
			ToModelOutput: func(_ string, _ weatherInput, _ weatherOutput) (*provider.ToolResultOutput, error) {
				return nil, nil
			},
		})
		require.NoError(t, err)

		_, err = tool.ToModelOutput(ToolOutputContext{
			ToolCallID: "call-1",
			Input:      json.RawMessage(`{"city":"Berlin"}`),
			Output:     json.RawMessage(`bad-json`),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aisdk: unmarshaling tool output for ToModelOutput")
	})

	t.Run("NilToModelOutput", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
		})
		require.NoError(t, err)
		assert.Nil(t, tool.ToModelOutput)
	})
}

func TestTypedTool_Execute(t *testing.T) {
	t.Run("SuccessfulRoundTrip", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
			Execute: func(_ context.Context, input weatherInput, _ ToolExecutionOptions) (weatherOutput, error) {
				return weatherOutput{Temp: 20.5, Unit: "C"}, nil
			},
		})
		require.NoError(t, err)

		result, err := tool.Execute(context.Background(), json.RawMessage(`{"city":"Berlin"}`), ToolExecutionOptions{})
		require.NoError(t, err)

		var out weatherOutput
		require.NoError(t, json.Unmarshal(result, &out))
		assert.Equal(t, 20.5, out.Temp)
		assert.Equal(t, "C", out.Unit)
	})

	t.Run("InputUnmarshalFailure", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
			Execute: func(_ context.Context, input weatherInput, _ ToolExecutionOptions) (weatherOutput, error) {
				return weatherOutput{}, nil
			},
		})
		require.NoError(t, err)

		_, err = tool.Execute(context.Background(), json.RawMessage(`not-valid-json`), ToolExecutionOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshaling tool input")
	})

	t.Run("ExecuteErrorPropagation", func(t *testing.T) {
		execErr := errors.New("city not found")
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
			Execute: func(_ context.Context, input weatherInput, _ ToolExecutionOptions) (weatherOutput, error) {
				return weatherOutput{}, execErr
			},
		})
		require.NoError(t, err)

		_, err = tool.Execute(context.Background(), json.RawMessage(`{"city":"Atlantis"}`), ToolExecutionOptions{})
		require.Error(t, err)
		assert.ErrorIs(t, err, execErr)
	})

	t.Run("OutputMarshalFailure", func(t *testing.T) {
		type badOutput struct {
			Value float64 `json:"value"`
		}

		tool, err := TypedTool(TypedToolDef[weatherInput, badOutput]{
			Description: "weather",
			Execute: func(_ context.Context, input weatherInput, _ ToolExecutionOptions) (badOutput, error) {
				return badOutput{Value: math.NaN()}, nil
			},
		})
		require.NoError(t, err)

		_, err = tool.Execute(context.Background(), json.RawMessage(`{"city":"test"}`), ToolExecutionOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aisdk: marshaling tool output")
	})

	t.Run("PassesOptions", func(t *testing.T) {
		var receivedOpts ToolExecutionOptions
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
			Execute: func(_ context.Context, input weatherInput, opts ToolExecutionOptions) (weatherOutput, error) {
				receivedOpts = opts
				return weatherOutput{}, nil
			},
		})
		require.NoError(t, err)

		opts := ToolExecutionOptions{
			ToolCallID: "call-123",
			Context:    "test-context",
		}
		_, err = tool.Execute(context.Background(), json.RawMessage(`{"city":"x"}`), opts)
		require.NoError(t, err)
		assert.Equal(t, "call-123", receivedOpts.ToolCallID)
		assert.Equal(t, "test-context", receivedOpts.Context)
	})
}

func TestTypedTool_ValidateInput(t *testing.T) {
	t.Run("ValidInput", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
			ValidateInput: func(input weatherInput) error {
				if input.City == "" {
					return errors.New("city required")
				}
				return nil
			},
		})
		require.NoError(t, err)

		err = tool.ValidateInput(json.RawMessage(`{"city":"London"}`))
		require.NoError(t, err)
	})

	t.Run("InvalidInput", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
			ValidateInput: func(input weatherInput) error {
				if input.City == "" {
					return errors.New("city required")
				}
				return nil
			},
		})
		require.NoError(t, err)

		err = tool.ValidateInput(json.RawMessage(`{"city":""}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "city required")
	})

	t.Run("UnmarshalFailure", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
			ValidateInput: func(input weatherInput) error {
				return nil
			},
		})
		require.NoError(t, err)

		err = tool.ValidateInput(json.RawMessage(`bad-json`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshaling tool input for validation")
	})

	t.Run("NilValidateInput", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
		})
		require.NoError(t, err)
		assert.Nil(t, tool.ValidateInput)
	})
}

func TestTypedTool_InputExamples(t *testing.T) {
	t.Run("MarshaledCorrectly", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description:   "weather",
			InputExamples: []weatherInput{{City: "London"}, {City: "Tokyo"}},
		})
		require.NoError(t, err)
		require.Len(t, tool.InputExamples, 2)

		var ex0 weatherInput
		require.NoError(t, json.Unmarshal(tool.InputExamples[0], &ex0))
		assert.Equal(t, "London", ex0.City)

		var ex1 weatherInput
		require.NoError(t, json.Unmarshal(tool.InputExamples[1], &ex1))
		assert.Equal(t, "Tokyo", ex1.City)
	})

	t.Run("EmptyExamples", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[weatherInput, weatherOutput]{
			Description: "weather",
		})
		require.NoError(t, err)
		assert.Nil(t, tool.InputExamples)
	})

	t.Run("MarshalFailure", func(t *testing.T) {
		type badExample struct {
			Value float64 `json:"value"`
		}

		_, err := TypedTool(TypedToolDef[badExample, string]{
			Description:   "bad examples",
			InputExamples: []badExample{{Value: math.NaN()}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aisdk: marshaling input example")
	})
}

func TestTypedTool_ToolSetIntegration(t *testing.T) {
	def := TypedToolDef[weatherInput, weatherOutput]{
		Name:        "weather",
		Description: "Get current weather",
		Execute: func(_ context.Context, input weatherInput, _ ToolExecutionOptions) (weatherOutput, error) {
			return weatherOutput{Temp: 72, Unit: "F"}, nil
		},
	}

	tool, err := TypedTool(def)
	require.NoError(t, err)

	tools := ToolSet{
		def.Name: tool,
	}

	assert.Contains(t, tools, "weather")
	assert.Equal(t, "Get current weather", tools["weather"].Description)
}
