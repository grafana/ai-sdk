package provider

import "encoding/json"

// Citation is a reference supporting generated text, attached independently of
// the complete set of retrieved sources.
type Citation struct {
	Source     SourceInfo `json:"source"`
	StartIndex *int       `json:"startIndex,omitempty"`
	EndIndex   *int       `json:"endIndex,omitempty"`
	CitedText  *string    `json:"citedText,omitempty"`
}

type citationSourceJSON struct {
	Type             ContentPartType  `json:"type"`
	SourceType       SourceType       `json:"sourceType"`
	ID               string           `json:"id"`
	URL              *string          `json:"url,omitempty"`
	MediaType        *string          `json:"mediaType,omitempty"`
	Title            *string          `json:"title,omitempty"`
	Filename         string           `json:"filename,omitempty"`
	ProviderMetadata ProviderMetadata `json:"providerMetadata,omitzero"`
}

// MarshalJSON encodes the source as a discriminated source object, keeping the
// fields each source type requires even when empty.
func (c Citation) MarshalJSON() ([]byte, error) {
	s := c.Source
	source := citationSourceJSON{Type: ContentPartTypeSource, SourceType: s.SourceType, ID: s.ID, ProviderMetadata: s.ProviderMetadata}
	switch s.SourceType {
	case SourceTypeURL:
		source.URL = &s.URL
		if s.Title != "" {
			source.Title = &s.Title
		}
	case SourceTypeDocument:
		source.MediaType, source.Title, source.Filename = &s.MediaType, &s.Title, s.Filename
	}
	return json.Marshal(struct {
		Source     citationSourceJSON `json:"source"`
		StartIndex *int               `json:"startIndex,omitempty"`
		EndIndex   *int               `json:"endIndex,omitempty"`
		CitedText  *string            `json:"citedText,omitempty"`
	}{source, c.StartIndex, c.EndIndex, c.CitedText})
}
