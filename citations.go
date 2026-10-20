package aisdk

import "github.com/grafana/ai-sdk/provider"

func cloneCitations(citations []provider.Citation) []provider.Citation {
	if citations == nil {
		return nil
	}
	result := make([]provider.Citation, len(citations))
	for i, citation := range citations {
		result[i] = citation
		result[i].Source.ProviderMetadata = cloneProviderMetadata(citation.Source.ProviderMetadata)
		if citation.StartIndex != nil {
			result[i].StartIndex = new(*citation.StartIndex)
		}
		if citation.EndIndex != nil {
			result[i].EndIndex = new(*citation.EndIndex)
		}
		if citation.CitedText != nil {
			result[i].CitedText = new(*citation.CitedText)
		}
	}
	return result
}
