package aisdk

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type toolPartJSON ToolInvocationPart

func marshalToolPart(p toolPartJSON) ([]byte, error) {
	var toolMetadata *map[string]json.RawMessage
	var callMetadata, resultMetadata *provider.ProviderMetadata
	if p.ToolMetadata != nil {
		toolMetadata = &p.ToolMetadata
	}
	if p.CallProviderMetadata != nil {
		callMetadata = &p.CallProviderMetadata
	}
	if p.ResultProviderMetadata != nil {
		resultMetadata = &p.ResultProviderMetadata
	}
	return json.Marshal(struct {
		toolPartJSON
		ToolMetadata           *map[string]json.RawMessage `json:"toolMetadata,omitempty"`
		CallProviderMetadata   *provider.ProviderMetadata  `json:"callProviderMetadata,omitempty"`
		ResultProviderMetadata *provider.ProviderMetadata  `json:"resultProviderMetadata,omitempty"`
	}{p, toolMetadata, callMetadata, resultMetadata})
}

func (p ToolInvocationPart) MarshalJSON() ([]byte, error) { return marshalToolPart(toolPartJSON(p)) }
func (p DynamicToolUIPart) MarshalJSON() ([]byte, error)  { return marshalToolPart(toolPartJSON(p)) }

func unmarshalToolPart(data []byte) (toolPartJSON, error) {
	var p toolPartJSON
	if err := rejectNullToolFields(data, "title", "toolMetadata", "errorText", "preliminary", "providerExecuted", "callProviderMetadata", "resultProviderMetadata", "approval"); err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	if p.State == ToolStateInputStreaming && p.RawInput != nil {
		var raw *string
		if err := json.Unmarshal(p.RawInput, &raw); err != nil || raw == nil {
			return p, fmt.Errorf("aisdk: streaming raw input must be a JSON string")
		}
	}
	for _, metadata := range []provider.ProviderMetadata{p.CallProviderMetadata, p.ResultProviderMetadata} {
		if err := validateProviderMetadata(metadata); err != nil {
			return p, err
		}
	}
	return p, nil
}

func (p *ToolInvocationPart) UnmarshalJSON(data []byte) error {
	decoded, err := unmarshalToolPart(data)
	if err != nil {
		return err
	}
	*p = ToolInvocationPart(decoded)
	return nil
}

func (p *DynamicToolUIPart) UnmarshalJSON(data []byte) error {
	decoded, err := unmarshalToolPart(data)
	if err != nil {
		return err
	}
	*p = DynamicToolUIPart(decoded)
	return nil
}

func (p *ToolApproval) UnmarshalJSON(data []byte) error {
	if err := rejectNullToolFields(data, "id", "approved", "requestReason", "reason", "isAutomatic", "signature"); err != nil {
		return err
	}
	type approval ToolApproval
	var decoded approval
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = ToolApproval(decoded)
	return nil
}

func validateProviderMetadata(metadata provider.ProviderMetadata) error {
	for namespace, raw := range metadata {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil || object == nil {
			return fmt.Errorf("aisdk: provider metadata namespace %q must be a JSON object", namespace)
		}
	}
	return nil
}

func isJSONNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

func rejectNullToolFields(data []byte, fields ...string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	for _, field := range fields {
		if raw, ok := object[field]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("aisdk: tool field %q must not be null", field)
		}
	}
	return nil
}
