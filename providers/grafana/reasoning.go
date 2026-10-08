package grafana

import (
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

func decodeReasoningFile(raw json.RawMessage) (*provider.StreamFileData, error) {
	var value struct {
		Type provider.StreamFileDataType `json:"type"`
		Data *string                     `json:"data"`
		URL  *string                     `json:"url"`
	}
	if decodeFields(raw, &value, "type", "data", "url") != nil {
		return nil, errors.New("grafana: invalid reasoning file")
	}
	file := &provider.StreamFileData{Type: value.Type}
	switch value.Type {
	case provider.StreamFileDataTypeData:
		if value.Data == nil || value.URL != nil {
			return nil, errors.New("grafana: invalid reasoning inline data")
		}
		file.Base64 = *value.Data
	case provider.StreamFileDataTypeURL:
		if value.URL == nil || value.Data != nil {
			return nil, errors.New("grafana: invalid reasoning URL")
		}
		file.URL = *value.URL
	default:
		return nil, errors.New("grafana: unsupported reasoning file arm")
	}
	if err := file.Validate(); err != nil {
		return nil, err
	}
	return file, nil
}
