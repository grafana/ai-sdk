package grafana

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/grafana/ai-sdk/provider"
)

var errInvalidResponseMetadata = errors.New("grafana: invalid provider metadata")

const acceptedCallerDirect = "direct"

func strictMetadataObject(data []byte) (map[string]json.RawMessage, error) {
	if !validJSON(data) {
		return nil, errInvalidResponseMetadata
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, errInvalidResponseMetadata
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, errInvalidResponseMetadata
		}
		name, ok := key.(string)
		if !ok {
			return nil, errInvalidResponseMetadata
		}
		if _, exists := fields[name]; exists {
			return nil, errInvalidResponseMetadata
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, errInvalidResponseMetadata
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, errInvalidResponseMetadata
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errInvalidResponseMetadata
	}
	return fields, nil
}

func decodeProviderMetadata(raw json.RawMessage, allowAnthropic bool, textID string) (provider.ProviderMetadata, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	fields, err := strictMetadataObject(raw)
	if err != nil || len(fields) == 0 {
		return nil, errInvalidResponseMetadata
	}
	mapped := make(provider.ProviderMetadata, len(fields))
	for namespace, value := range fields {
		entry, err := strictMetadataObject(value)
		if err != nil || len(entry) != 1 {
			return nil, errInvalidResponseMetadata
		}
		switch namespace {
		case "openai":
			item, ok := entry["itemId"]
			if !ok {
				return nil, errInvalidResponseMetadata
			}
			var id string
			if json.Unmarshal(item, &id) != nil || !validResponseItemID(id) || (textID != "" && id != textID) {
				return nil, errInvalidResponseMetadata
			}
			encoded, err := json.Marshal(struct {
				ItemID string `json:"itemId"`
			}{ItemID: id})
			if err != nil {
				return nil, errInvalidResponseMetadata
			}
			mapped[namespace] = encoded
		case "anthropic":
			if !allowAnthropic {
				return nil, errInvalidResponseMetadata
			}
			caller, ok := entry["caller"]
			if !ok {
				return nil, errInvalidResponseMetadata
			}
			inner, err := strictMetadataObject(caller)
			if err != nil || len(inner) != 1 {
				return nil, errInvalidResponseMetadata
			}
			var kind string
			if json.Unmarshal(inner["type"], &kind) != nil || kind != acceptedCallerDirect {
				return nil, errInvalidResponseMetadata
			}
			mapped[namespace] = json.RawMessage(`{"caller":{"type":"direct"}}`)
		default:
			return nil, errInvalidResponseMetadata
		}
	}
	return mapped, nil
}

func validResponseItemID(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '-' {
			return false
		}
	}
	return true
}
