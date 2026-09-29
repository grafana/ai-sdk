package v4

import (
	"encoding/json"
	"errors"
	"unicode/utf8"

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

type projectedToolCaller struct {
	Type     toolCallerKind `json:"type"`
	ToolID   string         `json:"toolId,omitempty"`
	CallerID string         `json:"callerId,omitempty"`
}

type projectedAnthropicToolMetadata struct {
	Caller *projectedToolCaller `json:"caller,omitempty"`
}

type projectedResponsesToolMetadata struct {
	ItemID    *string              `json:"itemId,omitempty"`
	Namespace *string              `json:"namespace,omitempty"`
	Caller    *projectedToolCaller `json:"caller,omitempty"`
}

func mapToolMetadata(metadata provider.ProviderMetadata, limit int64) (provider.ProviderMetadata, error) {
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
		var encoded json.RawMessage
		var err error
		if name == "anthropic" {
			encoded, err = projectAnthropicToolMetadata(raw)
		} else {
			encoded, err = projectResponsesToolMetadata(raw)
		}
		if err != nil || int64(len(encoded)) > limit {
			return nil, errInvalidToolMetadata
		}
		if len(encoded) == 0 {
			continue
		}
		if projected == nil {
			projected = make(provider.ProviderMetadata)
		}
		projected[name] = encoded
	}
	return projected, nil
}

func toolMetadataFields(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, errInvalidToolMetadata
	}
	return fields, nil
}

func projectAnthropicToolMetadata(raw json.RawMessage) (json.RawMessage, error) {
	fields, err := toolMetadataFields(raw)
	if err != nil {
		return nil, err
	}
	if len(fields["type"]) > 0 {
		kind, err := toolMetadataString(fields["type"])
		if err != nil || kind == "mcp-tool-use" {
			return nil, errInvalidToolMetadata
		}
	}
	if len(fields["caller"]) == 0 {
		return nil, nil
	}
	caller, err := projectToolCaller(fields["caller"], anthropicCaller)
	if err != nil {
		return nil, err
	}
	return json.Marshal(projectedAnthropicToolMetadata{Caller: caller})
}

func projectResponsesToolMetadata(raw json.RawMessage) (json.RawMessage, error) {
	fields, err := toolMetadataFields(raw)
	if err != nil {
		return nil, err
	}
	var selected projectedResponsesToolMetadata
	if len(fields["itemId"]) > 0 {
		itemID, err := toolMetadataString(fields["itemId"])
		if err != nil {
			return nil, err
		}
		selected.ItemID = &itemID
	}
	if len(fields["namespace"]) > 0 {
		namespace, err := toolMetadataString(fields["namespace"])
		if err != nil {
			return nil, err
		}
		selected.Namespace = &namespace
	}
	if len(fields["caller"]) > 0 {
		caller, err := projectToolCaller(fields["caller"], responsesCaller)
		if err != nil {
			return nil, err
		}
		selected.Caller = caller
	}
	if selected.ItemID == nil && selected.Namespace == nil && selected.Caller == nil {
		return nil, nil
	}
	return json.Marshal(selected)
}

func toolMetadataString(raw json.RawMessage) (string, error) {
	var value string
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
		return "", errInvalidToolMetadata
	}
	return value, nil
}

func projectToolCaller(raw json.RawMessage, family toolCallerFamily) (*projectedToolCaller, error) {
	fields, err := toolMetadataFields(raw)
	if err != nil {
		return nil, err
	}
	kind, err := toolMetadataString(fields["type"])
	if err != nil {
		return nil, err
	}
	caller := &projectedToolCaller{Type: toolCallerKind(kind)}
	switch caller.Type {
	case toolCallerDirect:
		if family == anthropicCaller && len(fields["toolId"]) > 0 || family == responsesCaller && len(fields["callerId"]) > 0 {
			return nil, errInvalidToolMetadata
		}
	case toolCallerCodeExecution2025, toolCallerCodeExecution2026:
		if family != anthropicCaller {
			return nil, errInvalidToolMetadata
		}
		caller.ToolID, err = toolMetadataString(fields["toolId"])
	case toolCallerProgram:
		if family != responsesCaller {
			return nil, errInvalidToolMetadata
		}
		caller.CallerID, err = toolMetadataString(fields["callerId"])
	default:
		return nil, errInvalidToolMetadata
	}
	if err != nil || caller.Type != toolCallerDirect && caller.ToolID == "" && caller.CallerID == "" {
		return nil, errInvalidToolMetadata
	}
	return caller, nil
}
