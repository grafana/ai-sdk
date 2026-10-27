package openai

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/grafana/ai-sdk/internal/jsonschema"
	"github.com/grafana/ai-sdk/provider"
)

const (
	propertyNamesWarningDetails     = "OpenAI does not support JSON Schema propertyNames. It was removed before sending the schema, so OpenAI will not enforce property-name constraints."
	lookaroundPatternWarningDetails = "OpenAI does not support regex lookaround in JSON Schema patterns. The pattern was removed before sending the schema, so OpenAI will not enforce that constraint."
)

type openAISchemaRemovals struct {
	propertyNames     bool
	lookaroundPattern bool
}

func normalizeOpenAIJSONSchema(raw json.RawMessage) (map[string]any, []provider.Warning, error) {
	schema, err := jsonschema.DecodeObject(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("openai: decoding JSON schema: %w", err)
	}
	if schema == nil {
		return nil, nil, fmt.Errorf("openai: JSON schema must be an object")
	}

	var removed openAISchemaRemovals
	if err := normalizeOpenAISchemaNode(schema, &removed); err != nil {
		return nil, nil, err
	}
	schema = expandOpenAIRootReference(schema)

	var warnings []provider.Warning
	if removed.propertyNames {
		warnings = append(warnings, provider.Warning{
			Type: provider.WarnCompatibility, Feature: "JSON Schema propertyNames", Details: propertyNamesWarningDetails,
		})
	}
	if removed.lookaroundPattern {
		warnings = append(warnings, provider.Warning{
			Type: provider.WarnCompatibility, Feature: "JSON Schema pattern with regex lookaround", Details: lookaroundPatternWarningDetails,
		})
	}
	return schema, warnings, nil
}

func normalizeOpenAISchemaNode(schema map[string]any, removed *openAISchemaRemovals) error {
	if names, ok := schema["propertyNames"]; ok {
		if names != nil {
			namedSchema, isSchema := names.(map[string]any)
			if !isSchema || namedSchema["type"] != "string" {
				return fmt.Errorf("openai: JSON Schema propertyNames that does not use a string schema")
			}
			removed.propertyNames = true
		}
		delete(schema, "propertyNames")
	}

	if pattern, ok := schema["pattern"].(string); ok && containsRegexLookaround(pattern) {
		delete(schema, "pattern")
		removed.lookaroundPattern = true
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

func normalizeOpenAISchemaArray(definitions []any, removed *openAISchemaRemovals) error {
	for i, definition := range definitions {
		if err := normalizeOpenAIDefinition(definition, removed); err != nil {
			return fmt.Errorf("openai: schema %d: %w", i, err)
		}
	}
	return nil
}

func normalizeOpenAIDefinition(definition any, removed *openAISchemaRemovals) error {
	switch value := definition.(type) {
	case bool:
		return nil
	case map[string]any:
		if err := normalizeOpenAISchemaNode(value, removed); err != nil {
			return err
		}
		if reference, ok := singletonOpenAIReference(value); ok {
			delete(value, "allOf")
			value["$ref"] = reference
		}
		return nil
	default:
		return fmt.Errorf("openai: invalid JSON schema definition %T", definition)
	}
}

func singletonOpenAIReference(schema map[string]any) (string, bool) {
	definitions, ok := schema["allOf"].([]any)
	if !ok || len(definitions) != 1 {
		return "", false
	}
	definition, ok := definitions[0].(map[string]any)
	if !ok || len(definition) != 1 {
		return "", false
	}
	reference, ok := definition["$ref"].(string)
	return reference, ok
}

func expandOpenAIRootReference(schema map[string]any) map[string]any {
	reference, ok := singletonOpenAIReference(schema)
	if !ok {
		return schema
	}
	definition := localOpenAIReferenceSchema(reference, schema)
	if definition == nil {
		return schema
	}

	siblings := maps.Clone(schema)
	delete(siblings, "allOf")
	expanded := maps.Clone(definition)
	maps.Copy(expanded, siblings)
	return expanded
}

func localOpenAIReferenceSchema(reference string, schema map[string]any) map[string]any {
	for _, keyword := range []string{"definitions", "$defs"} {
		name, ok := strings.CutPrefix(reference, "#/"+keyword+"/")
		if !ok || name == "" {
			continue
		}
		name = strings.ReplaceAll(name, "~1", "/")
		name = strings.ReplaceAll(name, "~0", "~")
		definitions, _ := schema[keyword].(map[string]any)
		definition, _ := definitions[name].(map[string]any)
		return definition
	}
	return nil
}

func containsRegexLookaround(pattern string) bool {
	escaped, inCharacterClass := false, false
	for index := 0; index < len(pattern); index++ {
		character := pattern[index]
		if escaped {
			escaped = false
			continue
		}
		switch character {
		case '\\':
			escaped = true
		case '[':
			inCharacterClass = true
		case ']':
			inCharacterClass = false
		case '(':
			if inCharacterClass {
				continue
			}
			suffix := pattern[index:]
			if strings.HasPrefix(suffix, "(?=") || strings.HasPrefix(suffix, "(?!") ||
				strings.HasPrefix(suffix, "(?<=") || strings.HasPrefix(suffix, "(?<!") {
				return true
			}
		}
	}
	return false
}
