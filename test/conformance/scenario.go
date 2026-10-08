//go:build conformance

package conformance

import (
	"context"
	"encoding/json"
	"fmt"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

type ScenarioResult struct {
	Chunks   []map[string]any `json:"chunks"`
	Usage    []provider.Usage `json:"usage"`
	Object   any              `json:"object"`
	Generate any              `json:"generate,omitempty"`
	Error    string           `json:"error,omitempty"`
}

func ExecuteScenario(ctx context.Context, cfg *Config, model provider.LanguageModel) (ScenarioResult, error) {
	capture := ScenarioResult{Chunks: []map[string]any{}, Usage: []provider.Usage{}}
	tools, err := cfg.BuildToolSet()
	if err != nil {
		return capture, fmt.Errorf("building tools: %w", err)
	}
	providerOpts, err := cfg.BuildProviderOptions()
	if err != nil {
		return capture, err
	}
	responseFormat, err := cfg.BuildResponseFormat()
	if err != nil {
		return capture, err
	}
	prompt := cfg.Prompt
	if prompt == "" {
		prompt = "test"
	}
	var messages []provider.Message
	if len(cfg.UIMessages) > 0 {
		uiMessages, buildErr := cfg.BuildUIMessages()
		if buildErr != nil {
			return capture, buildErr
		}
		messages, err = aisdk.ConvertToModelMessages(uiMessages, aisdk.WithTools(tools))
	} else {
		messages, err = cfg.BuildMessages(prompt)
	}
	if err != nil {
		return capture, err
	}
	if cfg.Operation == OperationGenerate {
		if cfg.System != "" {
			messages = append([]provider.Message{provider.NewSystemMessage(cfg.System)}, messages...)
		}
		result, callErr := model.DoGenerate(ctx, provider.CallOptions{
			Prompt: messages, ResponseFormat: responseFormat, Headers: cfg.Headers,
			ProviderOptions: provider.BuildProviderOptions(providerOpts...),
		})
		if callErr != nil {
			capture.Error = callErr.Error()
			return capture, nil
		}
		var metadata provider.ProviderMetadata
		if value, ok := result.ProviderMetadata["bedrock"]; ok {
			metadata = provider.ProviderMetadata{"bedrock": value}
		}
		capture.Generate = generateResultSnapshot{
			Content: result.Content, FinishReason: result.FinishReason, Usage: result.Usage,
			ProviderMetadata: metadata, Warnings: result.Warnings,
		}
		return capture, nil
	}
	out, err := cfg.BuildOutput()
	if err != nil {
		return capture, err
	}
	opts := cfg.buildStreamOptions(messages, tools, providerOpts, []aisdk.StopCondition{aisdk.StepCountIs(cfg.StopWhenStepCount)}, out, responseFormat)
	if cfg.MaxRetries != nil {
		opts = append(opts, aisdk.WithMaxRetries(*cfg.MaxRetries))
	}
	result := aisdk.StreamText(ctx, model, opts...)
	for chunk := range result.ToUIMessageStream(cfg.BuildUIMessageStreamOptions()...) {
		data, err := json.Marshal(chunk)
		if err != nil {
			return capture, err
		}
		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err != nil {
			return capture, err
		}
		capture.Chunks = append(capture.Chunks, parsed)
	}
	for _, step := range result.Steps() {
		capture.Usage = append(capture.Usage, step.Usage)
	}
	capture.Object = result.OutputValue()
	if err := result.Err(); err != nil {
		capture.Error = err.Error()
	}
	return capture, nil
}
