package v4

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

func metadataFits(metadata provider.ProviderMetadata, remaining *int64) bool {
	if metadata == nil {
		return true
	}
	if *remaining < 2 || int64(len(metadata)) > (*remaining-1)/6 {
		return false
	}
	*remaining -= 2
	separator := int64(3)
	for key, raw := range metadata {
		for _, cost := range []int64{separator, int64(len(key)), int64(len(raw))} {
			if cost > *remaining {
				return false
			}
			*remaining -= cost
		}
		separator = 4
	}
	return true
}

func mapMetadata(metadata provider.ProviderMetadata) (*provider.ProviderMetadata, error) {
	if metadata == nil {
		return nil, nil
	}
	for key, raw := range metadata {
		if !utf8.ValidString(key) || !utf8.Valid(raw) || !json.Valid(raw) {
			return nil, errInvalidUnarySuccess
		}
		object := bytes.TrimSpace(raw)
		if len(object) == 0 || object[0] != '{' {
			return nil, errInvalidUnarySuccess
		}
	}
	return &metadata, nil
}
