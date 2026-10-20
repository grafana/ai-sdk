package grafana

import (
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

func decodeCitations(raw json.RawMessage) ([]provider.Citation, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil || entries == nil {
		return nil, errors.New("grafana: invalid citations")
	}
	citations := make([]provider.Citation, 0, len(entries))
	for _, entry := range entries {
		var value struct {
			Source     json.RawMessage `json:"source"`
			StartIndex *int            `json:"startIndex"`
			EndIndex   *int            `json:"endIndex"`
			CitedText  *string         `json:"citedText"`
		}
		fields, err := decodeObject(entry)
		if err != nil || decodeSelectedFields(fields, &value, "source", "startIndex", "endIndex", "citedText") != nil {
			return nil, errors.New("grafana: invalid citation")
		}
		for _, key := range []string{"startIndex", "endIndex", "citedText"} {
			if string(fields[key]) == "null" {
				return nil, errors.New("grafana: invalid citation field")
			}
		}
		for _, index := range []*int{value.StartIndex, value.EndIndex} {
			if index != nil && (*index < 0 || int64(*index) > 9007199254740991) {
				return nil, errors.New("grafana: invalid citation index")
			}
		}
		var discriminator struct {
			Type provider.GenerateContentType `json:"type"`
		}
		if decodeFields(value.Source, &discriminator, "type") != nil || discriminator.Type != provider.ContentSource {
			return nil, errors.New("grafana: invalid citation source")
		}
		source, err := decodeSource(value.Source)
		if err != nil {
			return nil, err
		}
		citations = append(citations, provider.Citation{Source: *source, StartIndex: value.StartIndex, EndIndex: value.EndIndex, CitedText: value.CitedText})
	}
	return citations, nil
}
