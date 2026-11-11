package v4

import (
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

func validCitations(citations []provider.Citation) bool {
	for _, citation := range citations {
		if !validTokenCounts(citation.StartIndex, citation.EndIndex) || !validSourceFields(citation.Source) || citation.CitedText != nil && !utf8.ValidString(*citation.CitedText) {
			return false
		}
	}
	return true
}
