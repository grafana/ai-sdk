package aisdk

import (
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

const maxCitationIndex = 1<<53 - 1

func validateCitations(citations []provider.Citation) error {
	for _, citation := range citations {
		if citation.Source.SourceType != provider.SourceTypeURL && citation.Source.SourceType != provider.SourceTypeDocument {
			return fmt.Errorf("unknown citation source type %q", citation.Source.SourceType)
		}
		for _, index := range []*int{citation.StartIndex, citation.EndIndex} {
			if index != nil && (*index < 0 || int64(*index) > maxCitationIndex) {
				return fmt.Errorf("invalid citation index %d", *index)
			}
		}
		if err := validateProviderMetadata(citation.Source.ProviderMetadata); err != nil {
			return err
		}
	}
	return nil
}

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
