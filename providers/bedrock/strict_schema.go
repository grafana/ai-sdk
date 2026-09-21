package bedrock

import "encoding/json"

func strictToolSchemaCompatible(raw json.RawMessage) bool {
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return false
	}
	return strictSchemaCompatible(schema)
}

func strictSchemaCompatible(schema any) bool {
	switch node := schema.(type) {
	case bool:
		return true
	case map[string]any:
		isObject := node["type"] == "object"
		if types, ok := node["type"].([]any); ok {
			for _, kind := range types {
				if kind == "object" {
					isObject = true
				}
			}
		}
		if isObject && node["additionalProperties"] != false {
			return false
		}
		for _, keyword := range []string{"properties", "patternProperties", "definitions", "$defs"} {
			if schemas, ok := node[keyword].(map[string]any); ok {
				for _, nested := range schemas {
					if !strictSchemaCompatible(nested) {
						return false
					}
				}
			}
		}
		if dependencies, ok := node["dependencies"].(map[string]any); ok {
			for _, dependency := range dependencies {
				if _, names := dependency.([]any); !names && !strictSchemaCompatible(dependency) {
					return false
				}
			}
		}
		for _, keyword := range []string{"propertyNames", "contains", "not", "if", "then", "else", "items", "anyOf", "allOf", "oneOf"} {
			switch nested := node[keyword].(type) {
			case nil:
			case []any:
				for _, item := range nested {
					if !strictSchemaCompatible(item) {
						return false
					}
				}
			default:
				if !strictSchemaCompatible(nested) {
					return false
				}
			}
		}
		return true
	default:
		return false
	}
}
