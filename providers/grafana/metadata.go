package grafana

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

func decodeProviderMetadata(raw json.RawMessage) (provider.ProviderMetadata, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var metadata provider.ProviderMetadata
	if json.Unmarshal(raw, &metadata) != nil || metadata == nil {
		return nil, errors.New("grafana: invalid provider metadata")
	}
	if err := validateMetadataNamespaces(metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}

func validateMetadataNamespaces(metadata provider.ProviderMetadata) error {
	for _, value := range metadata {
		if !bytes.HasPrefix(bytes.TrimSpace(value), []byte("{")) {
			return errors.New("grafana: invalid provider metadata namespace")
		}
	}
	return nil
}
