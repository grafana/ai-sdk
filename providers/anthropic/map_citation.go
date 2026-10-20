package anthropic

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/grafana/ai-sdk/provider"
)

type citationLocationType string

const (
	citationWebSearchResultLocation citationLocationType = "web_search_result_location"
	citationSearchResultLocation    citationLocationType = "search_result_location"
)

func mapCitations(citations []json.RawMessage, documents []citationDocument) ([]provider.Citation, error) {
	if len(citations) == 0 {
		return nil, nil
	}
	result := make([]provider.Citation, 0, len(citations))
	for _, raw := range citations {
		var citation struct {
			Type          citationLocationType `json:"type"`
			URL           string               `json:"url"`
			Source        string               `json:"source"`
			Title         string               `json:"title"`
			CitedText     *string              `json:"cited_text"`
			DocumentIndex int                  `json:"document_index"`
			DocumentTitle *string              `json:"document_title"`
			FileID        *string              `json:"file_id"`
		}
		if err := json.Unmarshal(raw, &citation); err != nil {
			return nil, fmt.Errorf("decoding citation: %w", err)
		}
		source := provider.SourceInfo{ProviderMetadata: provider.ProviderMetadata{"anthropic": raw}}
		if citation.Type == citationWebSearchResultLocation || citation.Type == citationSearchResultLocation {
			url := citation.URL
			if citation.Type == citationSearchResultLocation {
				url = citation.Source
			}
			source.SourceType = provider.SourceTypeURL
			source.ID, source.URL, source.Title = url, url, citation.Title
		} else {
			source.SourceType = provider.SourceTypeDocument
			source.ID = strconv.Itoa(citation.DocumentIndex)
			source.MediaType = "text/plain"
			source.Title = "Document " + source.ID
			if citation.DocumentIndex >= 0 && citation.DocumentIndex < len(documents) {
				document := documents[citation.DocumentIndex]
				source.MediaType, source.Title, source.Filename = document.mediaType, document.title, document.filename
			}
			if citation.FileID != nil {
				source.ID = *citation.FileID
			}
			if citation.DocumentTitle != nil {
				source.Title = *citation.DocumentTitle
			}
		}
		result = append(result, provider.Citation{Source: source, CitedText: citation.CitedText})
	}
	return result, nil
}
