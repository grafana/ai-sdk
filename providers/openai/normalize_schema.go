package openai

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

func normalizeOpenAIJSONSchema(raw json.RawMessage) (map[string]any, []provider.Warning, error) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, nil, fmt.Errorf("openai: decoding JSON schema: %w", err)
	}
	removed, err := normalizeSchemaNode(schema)
	if err != nil {
		return nil, nil, err
	}
	var warnings []provider.Warning
	if removed {
		warnings = []provider.Warning{{
			Type:    provider.WarnCompatibility,
			Feature: "JSON Schema propertyNames",
			Details: "OpenAI does not support JSON Schema propertyNames. It was removed before sending the schema, so OpenAI will not enforce property-name constraints.",
		}}
	}
	return schema, warnings, nil
}

func normalizeSchemaNode(value any) (bool, error) {
	schema, ok := value.(map[string]any)
	if !ok {
		return false, nil
	}
	removed := false
	if propertyNames := schema["propertyNames"]; propertyNames != nil {
		definition, ok := propertyNames.(map[string]any)
		if !ok || definition["type"] != "string" {
			return false, fmt.Errorf("openai: unsupported JSON Schema propertyNames that does not use a string schema")
		}
		removed = true
	}
	delete(schema, "propertyNames")
	var nested []any
	for _, keyword := range []string{"properties", "patternProperties", "definitions", "$defs", "dependencies"} {
		if definitions, ok := schema[keyword].(map[string]any); ok {
			for _, definition := range definitions {
				nested = append(nested, definition)
			}
		}
	}
	for _, keyword := range []string{"additionalProperties", "additionalItems", "items", "contains", "not", "allOf", "anyOf", "oneOf", "if", "then", "else"} {
		switch definition := schema[keyword].(type) {
		case []any:
			nested = append(nested, definition...)
		default:
			nested = append(nested, definition)
		}
	}
	for _, definition := range nested {
		childRemoved, err := normalizeSchemaNode(definition)
		if err != nil {
			return false, err
		}
		removed = removed || childRemoved
	}
	return removed, nil
}
