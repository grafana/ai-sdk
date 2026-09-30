package grafana

import (
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

func decodeSource(data []byte) (*provider.SourceInfo, error) {
	var value struct {
		SourceType       provider.SourceType       `json:"sourceType"`
		ID               *string                   `json:"id"`
		URL              *string                   `json:"url"`
		Title            *string                   `json:"title"`
		MediaType        *string                   `json:"mediaType"`
		Filename         *string                   `json:"filename"`
		ProviderMetadata provider.ProviderMetadata `json:"providerMetadata"`
	}
	if !validJSON(data) || decodeFields(data, &value, "sourceType", "id", "url", "title", "mediaType", "filename", "providerMetadata") != nil || value.ID == nil {
		return nil, errors.New("grafana: invalid source")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return nil, errors.New("grafana: invalid source")
	}
	if _, present := fields["providerMetadata"]; present && value.ProviderMetadata == nil {
		return nil, errors.New("grafana: invalid source metadata")
	}
	source := &provider.SourceInfo{SourceType: value.SourceType, ID: *value.ID, ProviderMetadata: value.ProviderMetadata}
	switch value.SourceType {
	case provider.SourceTypeURL:
		if _, present := fields["title"]; present && value.Title == nil {
			return nil, errors.New("grafana: invalid source title")
		}
		if value.URL == nil {
			return nil, errors.New("grafana: missing source URL")
		}
		source.URL = *value.URL
		if value.Title != nil {
			source.Title = *value.Title
		}
	case provider.SourceTypeDocument:
		if _, present := fields["filename"]; present && value.Filename == nil {
			return nil, errors.New("grafana: invalid source filename")
		}
		if value.MediaType == nil || value.Title == nil {
			return nil, errors.New("grafana: missing document source fields")
		}
		source.MediaType, source.Title = *value.MediaType, *value.Title
		if value.Filename != nil {
			source.Filename = *value.Filename
		}
	default:
		return nil, errors.New("grafana: unsupported source type")
	}
	for _, raw := range value.ProviderMetadata {
		var namespace map[string]json.RawMessage
		if json.Unmarshal(raw, &namespace) != nil || namespace == nil {
			return nil, errors.New("grafana: invalid source metadata")
		}
	}
	return source, nil
}
