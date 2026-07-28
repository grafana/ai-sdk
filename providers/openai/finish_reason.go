package openai

import "github.com/grafana/ai-sdk/provider"

// mapFinishReason maps an OpenAI incomplete reason and the presence of a
// function call to a unified finish reason, mirroring upstream semantics.
func mapFinishReason(incompleteReason string, hasFunctionCall bool) provider.FinishReason {
	switch incompleteReason {
	case "":
		if hasFunctionCall {
			return provider.FinishReason{Unified: provider.FinishReasonToolCalls}
		}
		return provider.FinishReason{Unified: provider.FinishReasonStop}
	case "max_output_tokens":
		return provider.FinishReason{Unified: provider.FinishReasonLength, Raw: incompleteReason}
	case "content_filter":
		return provider.FinishReason{Unified: provider.FinishReasonContentFilter, Raw: incompleteReason}
	default:
		if hasFunctionCall {
			return provider.FinishReason{Unified: provider.FinishReasonToolCalls, Raw: incompleteReason}
		}
		return provider.FinishReason{Unified: provider.FinishReasonOther, Raw: incompleteReason}
	}
}
