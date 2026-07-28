package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

// ToolInputExampleFormatter formats a tool input example for a description.
type ToolInputExampleFormatter func(example provider.InputExample, index int) string

// AddToolInputExamplesOptions configures AddToolInputExamples.
type AddToolInputExamplesOptions struct {
	Prefix string
	Format ToolInputExampleFormatter
	Remove *bool
}

// AddToolInputExamples returns a Middleware that appends function-tool input
// examples to tool descriptions for providers that do not support examples.
func AddToolInputExamples(opts AddToolInputExamplesOptions) Middleware {
	if opts.Prefix == "" {
		opts.Prefix = "Input Examples:"
	}
	if opts.Format == nil {
		opts.Format = func(example provider.InputExample, _ int) string {
			var input any
			if err := json.Unmarshal(example.Input, &input); err != nil {
				return string(example.Input)
			}
			formatted, err := json.Marshal(input)
			if err != nil {
				return string(example.Input)
			}
			return string(formatted)
		}
	}
	remove := true
	if opts.Remove != nil {
		remove = *opts.Remove
	}
	return Middleware{
		TransformParams: func(_ context.Context, input TransformParamsInput) (provider.CallOptions, error) {
			params := input.Params
			if len(params.Tools) == 0 {
				return params, nil
			}
			tools := make([]provider.Tool, len(params.Tools))
			copy(tools, params.Tools)
			for i, tool := range tools {
				if tool.Type != provider.ToolTypeFunction || len(tool.InputExamples) == 0 {
					continue
				}
				formatted := make([]string, 0, len(tool.InputExamples))
				for j, example := range tool.InputExamples {
					if !json.Valid(example.Input) {
						return provider.CallOptions{}, fmt.Errorf("formatting tool input example %q[%d]: invalid JSON", tool.Name, j)
					}
					formatted = append(formatted, opts.Format(example, j))
				}
				examples := opts.Prefix + "\n" + strings.Join(formatted, "\n")
				if tool.Description != "" {
					tool.Description += "\n\n" + examples
				} else {
					tool.Description = examples
				}
				if remove {
					tool.InputExamples = nil
				}
				tools[i] = tool
			}
			params.Tools = tools
			return params, nil
		},
	}
}
