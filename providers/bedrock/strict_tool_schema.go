package bedrock

import "encoding/json"

func isStrictToolSchemaCompatible(raw json.RawMessage) bool {
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return false
	}
	return strictToolSchemaCompatible(schema)
}

func strictToolSchemaCompatible(value any) bool {
	schema, ok := value.(map[string]any)
	if !ok {
		_, boolean := value.(bool)
		return boolean
	}
	typeIsObject := schema["type"] == "object"
	if types, ok := schema["type"].([]any); ok {
		for _, typ := range types {
			typeIsObject = typeIsObject || typ == "object"
		}
	}
	if typeIsObject && schema["additionalProperties"] != false {
		return false
	}
	for _, key := range []string{"properties", "patternProperties", "definitions", "$defs", "dependencies"} {
		if children, ok := schema[key].(map[string]any); ok {
			for _, child := range children {
				if key == "dependencies" {
					if _, isArray := child.([]any); isArray {
						continue
					}
				}
				if !strictToolSchemaCompatible(child) {
					return false
				}
			}
		}
	}
	for _, key := range []string{"propertyNames", "contains", "not", "if", "then", "else", "items"} {
		if child, ok := schema[key]; ok {
			if values, array := child.([]any); array {
				for _, item := range values {
					if !strictToolSchemaCompatible(item) {
						return false
					}
				}
			} else if !strictToolSchemaCompatible(child) {
				return false
			}
		}
	}
	for _, key := range []string{"anyOf", "allOf", "oneOf"} {
		if children, ok := schema[key].([]any); ok {
			for _, child := range children {
				if !strictToolSchemaCompatible(child) {
					return false
				}
			}
		}
	}
	return true
}
