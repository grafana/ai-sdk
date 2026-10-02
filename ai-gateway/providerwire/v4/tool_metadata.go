package v4

import (
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

var errInvalidToolMetadata = errors.New("providerwire v4: invalid tool metadata")

type toolCallerKind string

const (
	toolCallerDirect            toolCallerKind = "direct"
	toolCallerCodeExecution2025 toolCallerKind = "code_execution_20250825"
	toolCallerCodeExecution2026 toolCallerKind = "code_execution_20260120"
	toolCallerProgram           toolCallerKind = "program"
)

type toolCallerFamily uint8

const (
	anthropicCaller toolCallerFamily = iota + 1
	responsesCaller
)

func validateToolMetadata(metadata provider.ProviderMetadata) error {
	if raw, exists := metadata["anthropic"]; exists {
		fields, err := toolMetadataFields(raw)
		if err != nil {
			return err
		}
		if value, exists := fields["type"]; exists {
			kind, err := toolMetadataString(value)
			if err != nil || kind == "mcp-tool-use" {
				return errInvalidToolMetadata
			}
		}
		if caller, exists := fields["caller"]; exists {
			if err := validateToolCaller(caller, anthropicCaller); err != nil {
				return err
			}
		}
	}
	for _, namespace := range []string{"openai", "azure"} {
		raw, exists := metadata[namespace]
		if !exists {
			continue
		}
		fields, err := toolMetadataFields(raw)
		if err != nil {
			return err
		}
		for _, name := range []string{"itemId", "namespace"} {
			if value, exists := fields[name]; exists {
				if _, err := toolMetadataString(value); err != nil {
					return err
				}
			}
		}
		if caller, exists := fields["caller"]; exists {
			if err := validateToolCaller(caller, responsesCaller); err != nil {
				return err
			}
		}
	}
	return nil
}

func toolMetadataFields(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, errInvalidToolMetadata
	}
	return fields, nil
}

func toolMetadataString(raw json.RawMessage) (string, error) {
	var value string
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return "", errInvalidToolMetadata
	}
	return value, nil
}

func validateToolCaller(raw json.RawMessage, family toolCallerFamily) error {
	fields, err := toolMetadataFields(raw)
	if err != nil {
		return err
	}
	value, err := toolMetadataString(fields["type"])
	if err != nil {
		return err
	}
	var idName string
	switch toolCallerKind(value) {
	case toolCallerDirect:
		if family == anthropicCaller && len(fields["toolId"]) > 0 || family == responsesCaller && len(fields["callerId"]) > 0 {
			return errInvalidToolMetadata
		}
		return nil
	case toolCallerCodeExecution2025, toolCallerCodeExecution2026:
		if family != anthropicCaller {
			return errInvalidToolMetadata
		}
		idName = "toolId"
	case toolCallerProgram:
		if family != responsesCaller {
			return errInvalidToolMetadata
		}
		idName = "callerId"
	default:
		return errInvalidToolMetadata
	}
	id, err := toolMetadataString(fields[idName])
	if err != nil || id == "" {
		return errInvalidToolMetadata
	}
	return nil
}
