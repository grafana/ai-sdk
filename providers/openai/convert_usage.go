package openai

import (
	ptrutil "github.com/grafana/ai-sdk/internal/ptr"
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
			Total:      ptrutil.To(inputTotal),
			NoCache:    ptrutil.To(noCache),
			CacheRead:  ptrutil.To(cached),
			CacheWrite: cacheWrite,
		},
		OutputTokens: provider.OutputTokenUsage{
			Total:     ptrutil.To(outputTotal),
			Text:      ptrutil.To(text),
			Reasoning: ptrutil.To(reasoning),
		},
		Raw: raw,
	}
}

func convertResponseUsage(u responses.ResponseUsage) provider.Usage {
	raw := []byte(u.RawJSON())
	if len(raw) == 0 {
		return provider.Usage{}
	}
	return convertUsage(u, raw)
}

func cacheWriteTokens(u responses.ResponseUsage) *int {
	if !u.InputTokensDetails.JSON.CacheWriteTokens.Valid() {
		return nil
	}
	return ptrutil.To(int(u.InputTokensDetails.CacheWriteTokens))
}
