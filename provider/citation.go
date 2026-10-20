package provider

import "encoding/json"

type Citation struct {
	Source     SourceInfo `json:"source"`
	StartIndex *int       `json:"startIndex,omitempty"`
	EndIndex   *int       `json:"endIndex,omitempty"`
	CitedText  *string    `json:"citedText,omitempty"`
}

func (c Citation) MarshalJSON() ([]byte, error) {
	source := map[string]any{
		"type":       ContentPartTypeSource,
		"sourceType": c.Source.SourceType,
		"id":         c.Source.ID,
	}
	switch c.Source.SourceType {
	case SourceTypeURL:
		source["url"] = c.Source.URL
		if c.Source.Title != "" {
			source["title"] = c.Source.Title
		}
	case SourceTypeDocument:
		source["mediaType"] = c.Source.MediaType
		source["title"] = c.Source.Title
		if c.Source.Filename != "" {
			source["filename"] = c.Source.Filename
		}
	}
	if c.Source.ProviderMetadata != nil {
		source["providerMetadata"] = c.Source.ProviderMetadata
	}
	return json.Marshal(struct {
		Source     map[string]any `json:"source"`
		StartIndex *int           `json:"startIndex,omitempty"`
		EndIndex   *int           `json:"endIndex,omitempty"`
		CitedText  *string        `json:"citedText,omitempty"`
	}{source, c.StartIndex, c.EndIndex, c.CitedText})
}
