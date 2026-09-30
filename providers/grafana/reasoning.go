package grafana

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

func decodeReasoningMetadata(raw json.RawMessage) (provider.ProviderMetadata, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var metadata map[string]json.RawMessage
	if json.Unmarshal(raw, &metadata) != nil || metadata == nil {
		return nil, errors.New("grafana: invalid reasoning metadata")
	}
	projected := make(provider.ProviderMetadata)
	for namespace, value := range metadata {
		if namespace != "anthropic" && namespace != "bedrock" && namespace != "amazonBedrock" && namespace != "openai" {
			continue
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(value, &fields) != nil || fields == nil {
			return nil, errors.New("grafana: invalid reasoning namespace")
		}
		selected := make(map[string]json.RawMessage)
		for key, field := range fields {
			allowed := (namespace == "anthropic" && (key == "signature" || key == "redactedData")) ||
				((namespace == "bedrock" || namespace == "amazonBedrock") && (key == "signature" || key == "redactedData" || key == "redactedContent")) ||
				(namespace == "openai" && (key == "itemId" || key == "reasoningEncryptedContent"))
			if !allowed {
				continue
			}
			var text *string
			if json.Unmarshal(field, &text) != nil || (text == nil && (namespace != "openai" || key != "reasoningEncryptedContent")) {
				return nil, errors.New("grafana: invalid reasoning continuation value")
			}
			selected[key] = field
		}
		encoded, err := json.Marshal(selected)
		if err != nil {
			return nil, err
		}
		projected[namespace] = encoded
	}
	return projected, nil
}

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
