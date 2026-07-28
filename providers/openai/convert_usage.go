package openai

import (
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

// convertUsage maps Responses usage to provider.Usage. Input tokens are split
// into noCache/cacheRead; output tokens into text/reasoning.
func convertUsage(u responses.ResponseUsage, raw []byte) provider.Usage {
	inputTotal := int(u.InputTokens)
	cached := int(u.InputTokensDetails.CachedTokens)
	cacheWrite := cacheWriteTokens(u)
	cacheWriteValue := 0
	if cacheWrite != nil {
		cacheWriteValue = *cacheWrite
	}
	noCache := inputTotal - cached - cacheWriteValue

	outputTotal := int(u.OutputTokens)
	reasoning := int(u.OutputTokensDetails.ReasoningTokens)
	text := outputTotal - reasoning

	return provider.Usage{
		InputTokens: provider.InputTokenUsage{
			Total:      intPtr(inputTotal),
			NoCache:    intPtr(noCache),
			CacheRead:  intPtr(cached),
			CacheWrite: cacheWrite,
		},
		OutputTokens: provider.OutputTokenUsage{
			Total:     intPtr(outputTotal),
			Text:      intPtr(text),
			Reasoning: intPtr(reasoning),
		},
		Raw: raw,
	}
}

func cacheWriteTokens(u responses.ResponseUsage) *int {
	if !u.InputTokensDetails.JSON.CacheWriteTokens.Valid() {
		return nil
	}
	return intPtr(int(u.InputTokensDetails.CacheWriteTokens))
}

func intPtr(v int) *int { return &v }
