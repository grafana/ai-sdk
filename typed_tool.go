package aisdk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

// TypedToolDef defines a tool using Go types instead of raw JSON.
// The input schema is derived from I via schema.SchemaFor[I]() and
// marshal/unmarshal is handled internally, so the Execute function
// works with typed input and output directly.
type TypedToolDef[I, O any] struct {
	Name        string
	Description string
	Title       string
	Execute     func(ctx context.Context, input I, opts ToolExecutionOptions) (O, error)

	OutputSchema    schema.Schema
	InputExamples   []I
	Strict          *bool
	ProviderOptions provider.ProviderOptions
	ValidateInput   func(input I) error
	ToModelOutput   func(toolCallID string, input I, output O) (*provider.ToolResultOutput, error)

	OnInputStart     func(ToolExecutionOptions)
	OnInputDelta     func(inputTextDelta string, opts ToolExecutionOptions)
	OnInputAvailable func(input I, err error, opts ToolExecutionOptions)
}

// TypedTool constructs a Tool from a typed definition. The input JSON Schema
// is derived from type I via schema.SchemaFor[I]() at construction time.
// The returned Tool.Execute wraps the typed execute function with automatic
// JSON unmarshal of input and marshal of output.
func TypedTool[I, O any](def TypedToolDef[I, O]) (tool Tool, err error) {
	defer func() {
		if r := recover(); r != nil {
			tool = Tool{}
			err = fmt.Errorf("aisdk: deriving input schema: %v", r)
		}
	}()

	inputSchema, err := schema.SchemaFor[I]()
	if err != nil {
		return Tool{}, fmt.Errorf("aisdk: deriving input schema: %w", err)
	}

	var examples []json.RawMessage
	for i, ex := range def.InputExamples {
		data, err := json.Marshal(ex)
		if err != nil {
			return Tool{}, fmt.Errorf("aisdk: marshaling input example %d: %w", i, err)
		}
		examples = append(examples, data)
	}

	tool = Tool{
		Description:     def.Description,
		Title:           def.Title,
		InputSchema:     inputSchema,
		OutputSchema:    def.OutputSchema,
		InputExamples:   examples,
		Strict:          def.Strict,
		ProviderOptions: def.ProviderOptions,

		OnInputStart: def.OnInputStart,
		OnInputDelta: def.OnInputDelta,
	}

	if def.ToModelOutput != nil {
		tool.ToModelOutput = func(ctx ToolOutputContext) (*provider.ToolResultOutput, error) {
			var input I
			if err := json.Unmarshal(ctx.Input, &input); err != nil {
				return nil, fmt.Errorf("aisdk: unmarshaling tool input for ToModelOutput: %w", err)
			}
			var output O
			if err := json.Unmarshal(ctx.Output, &output); err != nil {
				return nil, fmt.Errorf("aisdk: unmarshaling tool output for ToModelOutput: %w", err)
			}
			return def.ToModelOutput(ctx.ToolCallID, input, output)
		}
	}

	if def.OnInputAvailable != nil {
		tool.OnInputAvailable = func(raw json.RawMessage, opts ToolExecutionOptions) {
			var input I
			if err := json.Unmarshal(raw, &input); err != nil {
				def.OnInputAvailable(input, fmt.Errorf("aisdk: unmarshaling tool input: %w", err), opts)
				return
			}
			def.OnInputAvailable(input, nil, opts)
		}
	}

	if def.Execute != nil {
		tool.Execute = func(ctx context.Context, raw json.RawMessage, opts ToolExecutionOptions) (json.RawMessage, error) {
			var input I
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, fmt.Errorf("aisdk: unmarshaling tool input: %w", err)
			}
			out, err := def.Execute(ctx, input, opts)
			if err != nil {
				return nil, err
			}
			data, err := json.Marshal(out)
			if err != nil {
				return nil, fmt.Errorf("aisdk: marshaling tool output: %w", err)
			}
			return data, nil
		}
	}

	if def.ValidateInput != nil {
		tool.ValidateInput = func(raw json.RawMessage) error {
			var input I
			if err := json.Unmarshal(raw, &input); err != nil {
				return fmt.Errorf("aisdk: unmarshaling tool input for validation: %w", err)
			}
			return def.ValidateInput(input)
		}
	}

	return tool, nil
}
