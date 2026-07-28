package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
)

type anthropicIterationType string

const (
	anthropicIterationCompaction      anthropicIterationType = "compaction"
	anthropicIterationMessage         anthropicIterationType = "message"
	anthropicIterationFallbackMessage anthropicIterationType = "fallback_message"
)

type anthropicUsage struct {
	inputTokens              int64
	outputTokens             int64
	cacheCreationInputTokens int64
	cacheReadInputTokens     int64
	iterations               anthropic.BetaIterationsUsage
	raw                      json.RawMessage
}

func (a *streamAdapter) resetUsage(usage anthropic.BetaUsage) error {
	a.usage = anthropicUsage{
		inputTokens:              usage.InputTokens,
		outputTokens:             usage.OutputTokens,
		cacheCreationInputTokens: usage.CacheCreationInputTokens,
		cacheReadInputTokens:     usage.CacheReadInputTokens,
	}
	return a.mergeRawUsage(usage.RawJSON())
}

func (a *streamAdapter) updateUsage(usage anthropic.BetaMessageDeltaUsage) error {
	if usage.JSON.InputTokens.Valid() {
		a.usage.inputTokens = usage.InputTokens
	}
	a.usage.outputTokens = usage.OutputTokens
	if usage.JSON.CacheCreationInputTokens.Valid() {
		a.usage.cacheCreationInputTokens = usage.CacheCreationInputTokens
	}
	if usage.JSON.CacheReadInputTokens.Valid() {
		a.usage.cacheReadInputTokens = usage.CacheReadInputTokens
	}
	if usage.JSON.Iterations.Valid() {
		a.usage.iterations = usage.Iterations
	}
	return a.mergeRawUsage(usage.RawJSON())
}

func (a *streamAdapter) mergeRawUsage(raw string) error {
	merged := make(map[string]json.RawMessage)
	if len(a.usage.raw) > 0 {
		if err := json.Unmarshal(a.usage.raw, &merged); err != nil {
			return fmt.Errorf("unmarshaling accumulated usage: %w", err)
		}
	}
	if raw != "" {
		var update map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &update); err != nil {
			return fmt.Errorf("unmarshaling usage: %w", err)
		}
		for key, value := range update {
			merged[key] = value
		}
	}
	if len(merged) == 0 {
		a.usage.raw = nil
		return nil
	}
	data, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshaling accumulated usage: %w", err)
	}
	a.usage.raw = data
	return nil
}

func convertAnthropicUsage(usage anthropicUsage) provider.Usage {
	inputTokens := usage.inputTokens
	outputTokens := usage.outputTokens
	servedByFallback := false
	for _, iteration := range usage.iterations {
		if anthropicIterationType(iteration.Type) == anthropicIterationFallbackMessage {
			servedByFallback = true
			break
		}
	}

	if len(usage.iterations) > 0 && !servedByFallback {
		var executorInputTokens int64
		var executorOutputTokens int64
		executorIterations := 0
		for _, iteration := range usage.iterations {
			iterationType := anthropicIterationType(iteration.Type)
			if iterationType != anthropicIterationCompaction && iterationType != anthropicIterationMessage {
				continue
			}
			executorInputTokens += iteration.InputTokens
			executorOutputTokens += iteration.OutputTokens
			executorIterations++
		}
		if executorIterations > 0 {
			inputTokens = executorInputTokens
			outputTokens = executorOutputTokens
		}
	}

	noCache := int(inputTokens)
	cacheRead := int(usage.cacheReadInputTokens)
	cacheWrite := int(usage.cacheCreationInputTokens)
	totalInput := noCache + cacheRead + cacheWrite
	totalOutput := int(outputTokens)

	return provider.Usage{
		InputTokens: provider.InputTokenUsage{
			Total:      &totalInput,
			NoCache:    &noCache,
			CacheRead:  &cacheRead,
			CacheWrite: &cacheWrite,
		},
		OutputTokens: provider.OutputTokenUsage{Total: &totalOutput},
		Raw:          usage.raw,
	}
}
