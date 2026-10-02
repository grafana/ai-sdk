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
	const objectBytes = int64(len("{}"))
	const entryBytes = int64(len(`"":,`))
	namespaces := int64(len(metadata))
	if *remaining < objectBytes || namespaces > (*remaining-1)/(entryBytes+objectBytes) {
		return false
	}
	overhead := objectBytes
	if namespaces > 0 {
		overhead += namespaces*entryBytes - 1
	}
	*remaining -= overhead
	for key, raw := range metadata {
		for _, size := range []int{len(key), len(raw)} {
			if int64(size) > *remaining {
				return false
			}
			*remaining -= int64(size)
		}
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
