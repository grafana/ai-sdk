package anthropicschema

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// supportedStringFormats is the set of JSON Schema `format` values that
// Anthropic's structured-output decoder accepts. Other values are dropped
// from the sanitized schema and surfaced through the description appendix.
var supportedStringFormats = map[string]struct{}{
	"date-time": {},
	"time":      {},
	"date":      {},
	"duration":  {},
	"email":     {},
	"hostname":  {},
	"uri":       {},
	"ipv4":      {},
	"ipv6":      {},
	"uuid":      {},
}

// descriptionConstraintKeys lists JSON Schema validation keywords Anthropic
// rejects on output_config.format.schema. They are stripped from the
// sanitized schema and instead surfaced as a human-readable appendix
// appended to each node's description. Order is preserved in the appendix.
var descriptionConstraintKeys = []string{
	"minimum",
	"maximum",
	"exclusiveMinimum",
	"exclusiveMaximum",
	"multipleOf",
	"minLength",
	"maxLength",
	"pattern",
	"minItems",
	"maxItems",
	"uniqueItems",
	"minProperties",
	"maxProperties",
	"not",
}

// Sanitize returns a deep copy of schema with JSON Schema validation
// keywords that Anthropic's constrained decoder rejects stripped from every
// node. Removed numeric, string, array, and object cardinality constraints,
// the `not` keyword, and unsupported `format` values are reflected as
// human-readable text appended to each node's `description`. `oneOf` is
// rewritten as `anyOf`, `$ref` nodes short-circuit (siblings dropped), and
// object nodes always emit `additionalProperties: false`.
//
// The full original schema remains available for orchestration-layer result
// validation; this only relaxes the payload sent to Anthropic. The input map
// is not mutated.
func Sanitize(schema map[string]any) map[string]any {
	return sanitizeSchemaNode(schema)
}

func sanitizeSchemaNode(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	result := make(map[string]any)

	if ref, ok := schema["$ref"]; ok && ref != nil {
		result["$ref"] = ref
		return result
	}

	for _, key := range []string{"$schema", "$id", "title"} {
		if v, ok := schema[key]; ok && v != nil {
			result[key] = v
		}
	}

	if v, ok := schema["description"]; ok && v != nil {
		result["description"] = v
	}

	if v, ok := schema["default"]; ok {
		result["default"] = v
	}
	if v, ok := schema["const"]; ok {
		result["const"] = v
	}

	if v, ok := schema["enum"]; ok && v != nil {
		result["enum"] = v
	}

	if v, ok := schema["type"]; ok && v != nil {
		result["type"] = v
	}

	if v, ok := schema["anyOf"]; ok && v != nil {
		result["anyOf"] = sanitizeDefinitionList(v)
	} else if v, ok := schema["oneOf"]; ok && v != nil {
		result["anyOf"] = sanitizeDefinitionList(v)
	}
	if v, ok := schema["allOf"]; ok && v != nil {
		result["allOf"] = sanitizeDefinitionList(v)
	}

	if v, ok := schema["definitions"]; ok && v != nil {
		result["definitions"] = sanitizeDefinitionMap(v)
	}
	if v, ok := schema["$defs"]; ok && v != nil {
		result["$defs"] = sanitizeDefinitionMap(v)
	}

	typeIsObject := false
	if t, ok := schema["type"].(string); ok && t == "object" {
		typeIsObject = true
	}
	props, hasProps := schema["properties"].(map[string]any)
	if typeIsObject || hasProps {
		if hasProps {
			sanitized := make(map[string]any, len(props))
			for name, def := range props {
				sanitized[name] = sanitizeDefinition(def)
			}
			result["properties"] = sanitized
		}
		result["additionalProperties"] = false
		if req, ok := schema["required"]; ok && req != nil {
			result["required"] = req
		}
	}

	if v, ok := schema["items"]; ok && v != nil {
		switch items := v.(type) {
		case []any:
			out := make([]any, len(items))
			for i, def := range items {
				out[i] = sanitizeDefinition(def)
			}
			result["items"] = out
		default:
			result["items"] = sanitizeDefinition(items)
		}
	}

	if f, ok := schema["format"].(string); ok {
		if _, supported := supportedStringFormats[f]; supported {
			result["format"] = f
		}
	}

	appendix := constraintDescription(schema)
	if appendix == "" {
		return result
	}
	switch existing := result["description"].(type) {
	case nil:
		result["description"] = appendix
	case string:
		result["description"] = existing + "\n" + appendix
	default:
		result["description"] = fmt.Sprintf("%v\n%s", existing, appendix)
	}
	return result
}

func sanitizeDefinition(def any) any {
	if def == nil {
		return def
	}
	if _, ok := def.(bool); ok {
		return def
	}
	if m, ok := def.(map[string]any); ok {
		return sanitizeSchemaNode(m)
	}
	return def
}

func sanitizeDefinitionList(v any) []any {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]any, len(list))
	for i, def := range list {
		out[i] = sanitizeDefinition(def)
	}
	return out
}

func sanitizeDefinitionMap(v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, def := range m {
		out[k] = sanitizeDefinition(def)
	}
	return out
}

func constraintDescription(schema map[string]any) string {
	parts := make([]string, 0, len(descriptionConstraintKeys)+1)
	for _, key := range descriptionConstraintKeys {
		raw, ok := schema[key]
		if !ok || raw == nil {
			continue
		}
		if b, isBool := raw.(bool); isBool && !b {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", FormatConstraintName(key), FormatConstraintValue(raw)))
	}
	if f, ok := schema["format"].(string); ok {
		if _, supported := supportedStringFormats[f]; !supported {
			parts = append(parts, "format: "+f)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ") + "."
}

// FormatConstraintName converts a JSON Schema keyword into description text.
func FormatConstraintName(key string) string {
	var b strings.Builder
	b.Grow(len(key) + 4)
	for _, r := range key {
		if unicode.IsUpper(r) {
			b.WriteRune(' ')
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// FormatConstraintValue converts a JSON Schema constraint value into description text.
func FormatConstraintValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	bs, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(bs)
}
