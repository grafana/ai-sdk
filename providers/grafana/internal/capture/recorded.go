package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

func captureRecordedUI(ctx context.Context, model provider.LanguageModel, options provider.CallOptions, instructions string, results map[string][]json.RawMessage, steps int) error {
	if steps < 1 {
		return errors.New("invalid recorded step limit")
	}
	tools := make(aisdk.ToolSet, len(options.Tools))
	for _, definition := range options.Tools {
		if definition.Type != provider.ToolTypeFunction {
			return errors.New("unsupported recorded tool")
		}
		inputSchema, err := schema.SchemaFromJSON(definition.InputSchema)
		if err != nil {
			return err
		}
		tool := aisdk.Tool{Description: definition.Description, InputSchema: inputSchema, Strict: definition.Strict, ProviderOptions: definition.ProviderOptions}
		outputs := results[definition.Name]
		if len(outputs) > 0 {
			var mu sync.Mutex
			index := 0
			tool.Execute = func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
				mu.Lock()
				defer mu.Unlock()
				if index >= len(outputs) {
					return nil, errors.New("recorded mock results exhausted")
				}
				output := outputs[index]
				index++
				return output, nil
			}
		}
		tools[definition.Name] = tool
	}
	var ids atomic.Int64
	opts := []aisdk.StreamOption{aisdk.WithModelMessages(options.Prompt...), aisdk.WithTools(tools), aisdk.WithStopWhen(aisdk.StepCountIs(steps)), aisdk.WithMaxRetries(0), aisdk.WithGenerateID(func() string { return fmt.Sprintf("id-%d", ids.Add(1)-1) })}
	if instructions != "" {
		opts = append(opts, aisdk.WithSystem(instructions))
	}
	for _, option := range options.ProviderOptions {
		opts = append(opts, aisdk.WithProviderOptions(option))
	}
	result := aisdk.StreamText(ctx, model, opts...)
	var chunks []aisdk.UIMessageChunk
	for chunk := range result.ToUIMessageStream() {
		chunks = append(chunks, chunk)
	}
	if err := result.Err(); err != nil {
		return err
	}
	emit(map[string]any{"chunks": chunks})
	return nil
}
