package agentobservability

import (
	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/provider"
	"slices"
)

type generationChoice struct {
	index        int
	output       []agento11y.Message
	finishReason provider.FinishReason
}

func applyGenerationChoices(g *agento11y.Generation, choices []generationChoice) {
	if len(choices) == 0 {
		return
	}
	slices.SortFunc(choices, func(a, b generationChoice) int {
		if a.index < b.index {
			return -1
		}
		if a.index > b.index {
			return 1
		}
		return 0
	})
	g.Output = nil
	metadata := make([]map[string]any, 0, len(choices))
	finish := choices[0].finishReason
	for _, choice := range choices {
		metadata = append(metadata, map[string]any{"index": choice.index, "output_start": len(g.Output), "output_count": len(choice.output), "finish_reason": string(choice.finishReason.Unified)})
		g.Output = append(g.Output, choice.output...)
		if choice.finishReason.Unified != finish.Unified {
			finish = provider.FinishReason{Unified: provider.FinishReasonOther}
		}
	}
	g.StopReason = finishReasonToAgento11yStop(finish)
	g.Metadata = mergeMetadata(g.Metadata, map[string]any{"ai_sdk.choices": metadata})
}

func generateFinishReason(result *provider.GenerateResult) provider.FinishReason {
	if len(result.Choices) == 0 {
		return result.FinishReason
	}
	reason := result.Choices[0].FinishReason
	for _, choice := range result.Choices[1:] {
		if choice.FinishReason.Unified != reason.Unified {
			return provider.FinishReason{Unified: provider.FinishReasonOther}
		}
	}
	return reason
}
