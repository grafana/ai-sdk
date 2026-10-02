package grafana

import (
	"bytes"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

func decodeProviderMetadata(raw json.RawMessage) (provider.ProviderMetadata, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if !validJSON(raw) || bytes.TrimSpace(raw)[0] != '{' {
		return nil, errors.New("grafana: invalid provider metadata")
	}
	var metadata provider.ProviderMetadata
	if json.Unmarshal(raw, &metadata) != nil {
		return nil, errors.New("grafana: invalid provider metadata")
	}
	for name, value := range metadata {
		if !utf8.ValidString(name) || !validJSON(value) || bytes.TrimSpace(value)[0] != '{' {
			return nil, errors.New("grafana: invalid provider metadata namespace")
		}
	}
	return metadata, nil
}
