package aisdk

import (
	"context"

	"github.com/grafana/ai-sdk/provider"
)

// GenerateTextResult holds the complete result of a non-streaming LLM call.
type GenerateTextResult struct {
	Text             string
	Reasoning        []ReasoningOutput
	ToolCalls        []ToolCall
	ToolResults      []ToolResult
	Files            []GeneratedFile
	Sources          []Source
	FinishReason     provider.FinishReason
	Usage            provider.Usage
	TotalUsage       provider.Usage
	Steps            []StepResult
	Warnings         []provider.Warning
	Content          []ContentPart
	Response         ResponseMetadata
	ProviderMetadata provider.ProviderMetadata
	Output           any
	OutputError      error
}

// GenerateText performs a non-streaming LLM call. It runs StreamText
// internally and blocks until the stream completes, returning a
// populated result or the first error encountered.
func GenerateText(ctx context.Context, model provider.LanguageModel, opts ...GenerateOption) (*GenerateTextResult, error) {
	cfg := buildGenerateConfig(opts)
	result := streamTextWithConfig(ctx, model, cfg.toStreamConfig())

	for range result.FullStream() {
	}

	result.Wait()

	if err := result.Err(); err != nil {
		return nil, err
	}
	if err := result.abortError(); err != nil {
		return nil, err
	}

	gen := &GenerateTextResult{
		Text:             result.Text(),
		Reasoning:        result.Reasoning(),
		ToolCalls:        result.ToolCalls(),
		ToolResults:      result.ToolResults(),
		Files:            result.Files(),
		Sources:          result.Sources(),
		FinishReason:     result.FinishReason(),
		Usage:            result.Usage(),
		TotalUsage:       result.TotalUsage(),
		Steps:            result.Steps(),
		Warnings:         result.Warnings(),
		Content:          result.Content(),
		Response:         result.Response(),
		ProviderMetadata: result.ProviderMetadata(),
		Output:           result.OutputValue(),
		OutputError:      result.OutputError(),
	}

	return gen, nil
}
