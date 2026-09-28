package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

func buildAnthropicProviderMetadata(fields map[string]json.RawMessage, usageRaw json.RawMessage) (provider.ProviderMetadata, error) {
	var usage any = map[string]any{}
	if len(usageRaw) > 0 {
		if err := json.Unmarshal(usageRaw, &usage); err != nil {
			return nil, fmt.Errorf("unmarshaling usage metadata: %w", err)
		}
	}
	usageObject, _ := usage.(map[string]any)

	metadata := map[string]any{
		"usage":             usage,
		"stopSequence":      rawJSONValue(fields["stop_sequence"]),
		"iterations":        mapUsageIterations(usageObject["iterations"]),
		"container":         mapContainerMetadata(rawJSONValue(fields["container"])),
		"contextManagement": mapContextManagementMetadata(rawJSONValue(fields["context_management"])),
	}
	if stopDetails := mapStopDetailsMetadata(rawJSONValue(fields["stop_details"])); stopDetails != nil {
		metadata["stopDetails"] = stopDetails
	}
	if value := fields["safeguard_results"]; len(value) > 0 && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		results, err := mapSafeguardResults(value)
		if err != nil {
			return nil, err
		}
		metadata["safeguardResults"] = results
	}

	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshaling Anthropic provider metadata: %w", err)
	}
	return provider.ProviderMetadata{"anthropic": raw}, nil
}

func mapSafeguardResults(raw json.RawMessage) ([]map[string]any, error) {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil || entries == nil {
		return nil, fmt.Errorf("anthropic: invalid safeguard_results array")
	}
	results := make([]map[string]any, 0, len(entries))
	for i, entry := range entries {
		fields, ok := safeguardObject(entry)
		if !ok {
			return nil, fmt.Errorf("anthropic: invalid safeguard_results[%d] object", i)
		}
		kind, ok := safeguardString(fields, "type")
		if !ok {
			return nil, fmt.Errorf("anthropic: invalid safeguard_results[%d].type", i)
		}
		statusFields, ok := safeguardObject(fields["status"])
		if !ok {
			return nil, fmt.Errorf("anthropic: invalid safeguard_results[%d].status", i)
		}
		statusType, ok := safeguardString(statusFields, "type")
		if !ok {
			return nil, fmt.Errorf("anthropic: invalid safeguard_results[%d].status.type", i)
		}
		status := map[string]any{"type": statusType}
		if toolUses, exists := statusFields["tool_uses"]; exists {
			if bytes.Equal(bytes.TrimSpace(toolUses), []byte("null")) {
				status["tool_uses"] = nil
			} else {
				uses, ok := safeguardObject(toolUses)
				if !ok {
					return nil, fmt.Errorf("anthropic: invalid safeguard_results[%d].status.tool_uses", i)
				}
				mapped := make(map[string]any, len(uses))
				for id, rawUse := range uses {
					use, ok := safeguardObject(rawUse)
					if !ok {
						return nil, fmt.Errorf("anthropic: invalid safeguard_results[%d].status.tool_uses entry", i)
					}
					useType, ok := safeguardString(use, "type")
					if !ok {
						return nil, fmt.Errorf("anthropic: invalid safeguard_results[%d].status.tool_uses type", i)
					}
					verdict := map[string]any{"type": useType}
					for _, key := range []string{"outcome", "explanation"} {
						if value, exists := use[key]; exists {
							if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
								verdict[key] = nil
							} else if text, ok := safeguardString(use, key); ok {
								verdict[key] = text
							} else {
								return nil, fmt.Errorf("anthropic: invalid safeguard_results[%d].status.tool_uses %s", i, key)
							}
						}
					}
					mapped[id] = verdict
				}
				status["tool_uses"] = mapped
			}
		}
		results = append(results, map[string]any{"type": kind, "status": status})
	}
	return results, nil
}

func safeguardObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var object map[string]json.RawMessage
	err := json.Unmarshal(raw, &object)
	return object, err == nil && object != nil
}

func safeguardString(fields map[string]json.RawMessage, key string) (string, bool) {
	raw := bytes.TrimSpace(fields[key])
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func messageMetadataFields(raw string) map[string]json.RawMessage {
	var message map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &message)
	return message
}

func mergeMessageDeltaMetadata(fields map[string]json.RawMessage, raw string) map[string]json.RawMessage {
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	var event map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &event) != nil {
		return fields
	}
	var delta map[string]json.RawMessage
	if json.Unmarshal(event["delta"], &delta) == nil {
		for _, key := range []string{"stop_sequence", "stop_details", "container"} {
			value, ok := delta[key]
			if !ok {
				value = json.RawMessage("null")
			}
			fields[key] = value
		}
	}
	if value, ok := event["context_management"]; ok {
		fields["context_management"] = value
	}
	return fields
}

func rawJSONValue(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}

func mapUsageIterations(value any) any {
	iterations, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]any, 0, len(iterations))
	for _, value := range iterations {
		iteration, ok := value.(map[string]any)
		if !ok {
			continue
		}
		mapped := map[string]any{
			"type":         iteration["type"],
			"inputTokens":  iteration["input_tokens"],
			"outputTokens": iteration["output_tokens"],
		}
		if model, ok := iteration["model"].(string); ok {
			mapped["model"] = model
		}
		if tokens, ok := nonzeroJSONNumber(iteration["cache_creation_input_tokens"]); ok {
			mapped["cacheCreationInputTokens"] = tokens
		}
		if tokens, ok := nonzeroJSONNumber(iteration["cache_read_input_tokens"]); ok {
			mapped["cacheReadInputTokens"] = tokens
		}
		result = append(result, mapped)
	}
	return result
}

func mapStopDetailsMetadata(value any) map[string]any {
	details, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	mapped := map[string]any{"type": details["type"]}
	for _, field := range []struct {
		from string
		to   string
	}{
		{from: "category", to: "category"},
		{from: "explanation", to: "explanation"},
		{from: "recommended_model", to: "recommendedModel"},
	} {
		if value, ok := details[field.from].(string); ok {
			mapped[field.to] = value
		}
	}
	return mapped
}

func mapContainerMetadata(value any) any {
	container, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	mapped := map[string]any{
		"expiresAt": container["expires_at"],
		"id":        container["id"],
		"skills":    nil,
	}
	if skills, ok := container["skills"].([]any); ok {
		mappedSkills := make([]any, 0, len(skills))
		for _, value := range skills {
			skill, ok := value.(map[string]any)
			if !ok {
				continue
			}
			mappedSkills = append(mappedSkills, map[string]any{
				"type":    skill["type"],
				"skillId": skill["skill_id"],
				"version": skill["version"],
			})
		}
		mapped["skills"] = mappedSkills
	}
	return mapped
}

func mapContextManagementMetadata(value any) any {
	contextManagement, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	applied, _ := contextManagement["applied_edits"].([]any)
	mapped := make([]any, 0, len(applied))
	for _, value := range applied {
		edit, ok := value.(map[string]any)
		if !ok {
			continue
		}
		entry := map[string]any{"type": edit["type"]}
		switch edit["type"] {
		case "clear_tool_uses_20250919":
			entry["clearedToolUses"] = edit["cleared_tool_uses"]
			entry["clearedInputTokens"] = edit["cleared_input_tokens"]
		case "clear_thinking_20251015":
			entry["clearedThinkingTurns"] = edit["cleared_thinking_turns"]
			entry["clearedInputTokens"] = edit["cleared_input_tokens"]
		case "compact_20260112":
		default:
			continue
		}
		mapped = append(mapped, entry)
	}
	return map[string]any{"appliedEdits": mapped}
}

func nonzeroJSONNumber(value any) (any, bool) {
	switch value := value.(type) {
	case float64:
		return value, value != 0
	case json.Number:
		return value, value != "0"
	default:
		return nil, false
	}
}
