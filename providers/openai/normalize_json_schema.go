package openai

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

const propertyNamesWarningDetails = "OpenAI does not support JSON Schema propertyNames. It was removed before sending the schema, so OpenAI will not enforce property-name constraints."

func normalizeOpenAIJSONSchema(raw json.RawMessage) (map[string]any, []provider.Warning, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, nil, fmt.Errorf("openai: decoding JSON schema: %w", err)
	}
	if schema == nil {
		return nil, nil, fmt.Errorf("openai: JSON schema must be an object")
	}

	removed := false
	if err := normalizeOpenAISchemaNode(schema, &removed); err != nil {
		return nil, nil, err
	}
	if !removed {
		return schema, nil, nil
	}
	return schema, []provider.Warning{{
		Type:    provider.WarnCompatibility,
		Feature: "JSON Schema propertyNames",
		Details: propertyNamesWarningDetails,
	}}, nil
}

func normalizeOpenAISchemaNode(schema map[string]any, removed *bool) error {
	if names, ok := schema["propertyNames"]; ok {
		if names != nil {
			namedSchema, isSchema := names.(map[string]any)
			if !isSchema || namedSchema["type"] != "string" {
				return fmt.Errorf("openai: JSON Schema propertyNames that does not use a string schema")
			}
			*removed = true
		}
		delete(schema, "propertyNames")
	}

	for _, keyword := range []string{"properties", "patternProperties", "definitions", "$defs", "dependencies"} {
		record, ok := schema[keyword].(map[string]any)
		if !ok {
			continue
		}
		for key, definition := range record {
			if keyword == "dependencies" {
				if _, isNames := definition.([]any); isNames {
					continue
				}
			}
			if err := normalizeOpenAIDefinition(definition, removed); err != nil {
				return fmt.Errorf("openai: %s.%s: %w", keyword, key, err)
			}
		}
	}

	for _, keyword := range []string{"additionalProperties", "additionalItems", "contains", "not", "if", "then", "else"} {
		if definition := schema[keyword]; definition != nil {
			if err := normalizeOpenAIDefinition(definition, removed); err != nil {
				return fmt.Errorf("openai: %s: %w", keyword, err)
			}
		}
	}

	if items := schema["items"]; items != nil {
		if definitions, isArray := items.([]any); isArray {
			if err := normalizeOpenAISchemaArray(definitions, removed); err != nil {
				return fmt.Errorf("openai: items: %w", err)
			}
		} else if err := normalizeOpenAIDefinition(items, removed); err != nil {
			return fmt.Errorf("openai: items: %w", err)
		}
	}

	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		definitions, ok := schema[keyword].([]any)
		if !ok {
			continue
		}
		if err := normalizeOpenAISchemaArray(definitions, removed); err != nil {
			return fmt.Errorf("openai: %s: %w", keyword, err)
		}
	}
	return nil
}

func normalizeOpenAISchemaArray(definitions []any, removed *bool) error {
	for i, definition := range definitions {
		if err := normalizeOpenAIDefinition(definition, removed); err != nil {
			return fmt.Errorf("openai: schema %d: %w", i, err)
		}
	}
	return nil
}

func normalizeOpenAIDefinition(definition any, removed *bool) error {
	switch value := definition.(type) {
	case bool:
		return nil
	case map[string]any:
		return normalizeOpenAISchemaNode(value, removed)
	default:
		return fmt.Errorf("openai: invalid JSON schema definition %T", definition)
	}
}
