package v4

import (
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

const minimumCitationBytes = int64(len(`{"source":{"type":"source","sourceType":"url","id":"","url":""}}`))

func citationsPreflight(citations []provider.Citation, remaining *int64) bool {
	if int64(len(citations)) > *remaining/minimumCitationBytes {
		return false
	}
	*remaining -= int64(len(citations)) * minimumCitationBytes
	for _, citation := range citations {
		if !sourcePreflightBudget(citation.Source, remaining) || citation.CitedText != nil && !responseStringsFit(remaining, *citation.CitedText) {
			return false
		}
	}
	return true
}

func validCitations(citations []provider.Citation) bool {
	for _, citation := range citations {
		if !validTokenCounts(citation.StartIndex, citation.EndIndex) || !validSourceFields(citation.Source) || citation.CitedText != nil && !utf8.ValidString(*citation.CitedText) {
			return false
		}
	}
	return true
}
