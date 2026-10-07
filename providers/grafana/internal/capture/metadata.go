package main

import (
	"context"
	"encoding/json"
	"errors"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func unaryResponseMessages(content []provider.GenerateContentPart) ([]provider.Message, error) {
	parts := make([]provider.ContentPart, 0, len(content))
	for _, part := range content {
		switch part.Type {
		case provider.ContentSource:
			continue
		case provider.ContentText, provider.ContentReasoning, provider.ContentReasoningFile, provider.ContentToolCall:
		default:
			return nil, errors.New("unsupported capture content")
		}
		var options provider.ProviderOptions
		if part.ProviderMetadata != nil {
			options = make(provider.ProviderOptions, len(part.ProviderMetadata))
			for key, raw := range part.ProviderMetadata {
				options[key] = provider.RawProviderOption{Key: key, Raw: raw}
			}
		}
		parts = append(parts, provider.ContentPart{Type: provider.ContentPartType(part.Type), Text: part.Text, Data: part.Data, MediaType: part.MediaType, ToolCallID: part.ToolCallID, ToolName: part.ToolName, Input: part.Input, ProviderOptions: options})
	}
	return aisdk.ToResponseMessages(parts), nil
}

func captureMetadataReplay(ctx context.Context, model provider.LanguageModel, options provider.CallOptions, streaming bool) error {
	executions := 0
	execute := func(_ context.Context, input struct {
		City string `json:"city"`
	}, _ aisdk.ToolExecutionOptions) (string, error) {
		if input.City != "Rio" {
			return "", errors.New("unexpected capture tool input")
		}
		executions++
		return "sunny", nil
	}
	tool, err := aisdk.TypedTool(aisdk.TypedToolDef[struct {
		City string `json:"city"`
	}, string]{Name: "weather", Execute: execute})
	if err != nil {
		return err
	}
	if streaming {
		opts := []aisdk.StreamOption{aisdk.WithModelMessages(options.Prompt...), aisdk.WithTools(aisdk.ToolSet{"weather": tool}), aisdk.WithStopWhen(aisdk.StepCountIs(2)), aisdk.WithMaxRetries(0)}
		if options.MaxOutputTokens != nil {
			opts = append(opts, aisdk.WithMaxOutputTokens(*options.MaxOutputTokens))
		}
		for _, option := range options.ProviderOptions {
			opts = append(opts, aisdk.WithProviderOptions(option))
		}
		result := aisdk.StreamText(ctx, model, opts...)
		for range result.FullStream() {
		}
		if err := result.Err(); err != nil {
			return err
		}
		emit(map[string]any{"text": result.Text(), "executions": executions, "steps": len(result.Steps()), "messages": result.Response().Messages})
		return nil
	}
	first, err := model.DoGenerate(ctx, options)
	if err != nil {
		return err
	}
	messages, err := unaryResponseMessages(first.Content)
	if err != nil {
		return err
	}
	var results []provider.ContentPart
	for _, part := range first.Content {
		if part.Type != provider.ContentToolCall {
			continue
		}
		var input struct {
			City string `json:"city"`
		}
		if err := json.Unmarshal(part.Input, &input); err != nil {
			return err
		}
		output, err := execute(ctx, input, aisdk.ToolExecutionOptions{})
		if err != nil {
			return err
		}
		results = append(results, provider.ContentPart{Type: provider.ContentPartTypeToolResult, ToolCallID: part.ToolCallID, ToolName: part.ToolName, Output: &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: output}})
	}
	messages = append(messages, aisdk.ToResponseMessages(results)...)
	options.Prompt = append(append([]provider.Message{}, options.Prompt...), messages...)
	if _, err := model.DoGenerate(ctx, options); err != nil {
		return err
	}
	emit(map[string]any{"executions": executions, "messages": messages, "resultMetadata": first.ProviderMetadata})
	return nil
}
