package v4

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

var errInvalidToolMetadata = errors.New("providerwire v4: invalid tool metadata")

func mapToolMetadata(metadata provider.ProviderMetadata, mcpNames map[string]bool, limit int64) (provider.ProviderMetadata, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	if limit <= 0 || int64(len(metadata)) > limit {
		return nil, errInvalidToolMetadata
	}
	remaining := limit
	for name, raw := range metadata {
		if int64(len(name)) > remaining {
			return nil, errInvalidToolMetadata
		}
		remaining -= int64(len(name))
		if int64(len(raw)) > remaining {
			return nil, errInvalidToolMetadata
		}
		remaining -= int64(len(raw))
	}
	var projected provider.ProviderMetadata
	for _, name := range []string{"anthropic", "openai", "azure"} {
		raw, exists := metadata[name]
		if !exists {
			continue
		}
		if !utf8.Valid(raw) {
			return nil, errInvalidToolMetadata
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return nil, errInvalidToolMetadata
		}
		selected := make(map[string]any)
		if name == "anthropic" {
			mcpToolUse := false
			if value, ok := fields["type"]; ok {
				kind, err := toolMetadataString(value)
				if err != nil {
					return nil, err
				}
				if kind == "mcp-tool-use" {
					mcpToolUse = true
					serverName, err := toolMetadataString(fields["serverName"])
					if err != nil || !mcpNames[serverName] {
						return nil, errInvalidToolMetadata
					}
					selected["type"], selected["serverName"] = kind, serverName
				}
			}
			if value, ok := fields["caller"]; ok && !mcpToolUse {
				caller, err := mapToolCallerMetadata(value, "toolId", "code_execution_20250825", "code_execution_20260120")
				if err != nil {
					return nil, err
				}
				selected["caller"] = caller
			}
		} else {
			for _, field := range []string{"itemId", "namespace"} {
				if value, ok := fields[field]; ok {
					text, err := toolMetadataString(value)
					if err != nil {
						return nil, err
					}
					selected[field] = text
				}
			}
			if value, ok := fields["caller"]; ok {
				caller, err := mapToolCallerMetadata(value, "callerId", "program")
				if err != nil {
					return nil, err
				}
				selected["caller"] = caller
			}
		}
		if len(selected) == 0 {
			continue
		}
		encoded, err := json.Marshal(selected)
		if err != nil || int64(len(encoded)) > limit {
			return nil, errInvalidToolMetadata
		}
		if projected == nil {
			projected = make(provider.ProviderMetadata)
		}
		projected[name] = encoded
	}
	return projected, nil
}

func toolMetadataString(raw json.RawMessage) (string, error) {
	var value string
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return "", errInvalidToolMetadata
	}
	return value, nil
}

func mapToolCallerMetadata(raw json.RawMessage, idName string, variants ...string) (map[string]string, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, errInvalidToolMetadata
	}
	kind, err := toolMetadataString(fields["type"])
	if err != nil {
		return nil, err
	}
	valid := kind == "direct"
	for _, variant := range variants {
		valid = valid || kind == variant
	}
	if !valid {
		return nil, errInvalidToolMetadata
	}
	selected := map[string]string{"type": kind}
	if rawID, ok := fields[idName]; ok {
		if kind == "direct" {
			return nil, errInvalidToolMetadata
		}
		id, err := toolMetadataString(rawID)
		if err != nil {
			return nil, err
		}
		selected[idName] = id
	}
	return selected, nil
}
