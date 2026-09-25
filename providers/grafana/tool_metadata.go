package grafana

import (
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

var errToolMetadata = errors.New("grafana: invalid tool metadata")

func validToolFlags(raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	for _, name := range []string{"providerExecuted", "dynamic", "preliminary", "isError"} {
		if value, ok := fields[name]; ok {
			var marker bool
			if string(value) == "null" || json.Unmarshal(value, &marker) != nil {
				return false
			}
		}
	}
	return true
}

func decodeToolMetadata(raw json.RawMessage) (provider.ProviderMetadata, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var namespaces map[string]json.RawMessage
	if json.Unmarshal(raw, &namespaces) != nil || namespaces == nil {
		return nil, errToolMetadata
	}
	var result provider.ProviderMetadata
	for _, name := range []string{"anthropic", "openai", "azure"} {
		value, exists := namespaces[name]
		if !exists {
			continue
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(value, &fields) != nil || fields == nil {
			return nil, errToolMetadata
		}
		selected := make(map[string]any)
		if name == "anthropic" {
			mcpToolUse := false
			if rawType, ok := fields["type"]; ok {
				var kind string
				if json.Unmarshal(rawType, &kind) != nil {
					return nil, errToolMetadata
				}
				if kind == "mcp-tool-use" {
					mcpToolUse = true
					serverName, err := metadataString(fields, "serverName")
					if err != nil || serverName == "" {
						return nil, errToolMetadata
					}
					selected["type"] = kind
					selected["serverName"] = serverName
				}
			}
			if caller, ok := fields["caller"]; ok && !mcpToolUse {
				mapped, err := decodeToolCaller(caller, "toolId", "code_execution_20250825", "code_execution_20260120")
				if err != nil {
					return nil, err
				}
				selected["caller"] = mapped
			}
		} else {
			for _, key := range []string{"itemId", "namespace"} {
				if _, ok := fields[key]; ok {
					text, err := metadataString(fields, key)
					if err != nil {
						return nil, err
					}
					selected[key] = text
				}
			}
			if caller, ok := fields["caller"]; ok {
				mapped, err := decodeToolCaller(caller, "callerId", "program")
				if err != nil {
					return nil, err
				}
				selected["caller"] = mapped
			}
		}
		if len(selected) == 0 {
			continue
		}
		encoded, err := json.Marshal(selected)
		if err != nil {
			return nil, errToolMetadata
		}
		if result == nil {
			result = make(provider.ProviderMetadata)
		}
		result[name] = encoded
	}
	return result, nil
}

func metadataString(fields map[string]json.RawMessage, key string) (string, error) {
	var text string
	if json.Unmarshal(fields[key], &text) != nil || string(fields[key]) == "null" {
		return "", errToolMetadata
	}
	return text, nil
}

func decodeToolCaller(raw json.RawMessage, identifier string, permitted ...string) (map[string]string, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, errToolMetadata
	}
	kind, err := metadataString(fields, "type")
	if err != nil {
		return nil, err
	}
	valid := kind == "direct"
	for _, allowed := range permitted {
		valid = valid || kind == allowed
	}
	if !valid {
		return nil, errToolMetadata
	}
	result := map[string]string{"type": kind}
	if _, ok := fields[identifier]; ok {
		if kind == "direct" {
			return nil, errToolMetadata
		}
		id, err := metadataString(fields, identifier)
		if err != nil {
			return nil, err
		}
		result[identifier] = id
	}
	return result, nil
}
