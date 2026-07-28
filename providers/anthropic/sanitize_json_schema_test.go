package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jsonMap is a tiny helper that round-trips a JSON literal into the
// map[string]any shape the sanitizer expects. Using JSON keeps the test
// input syntactically close to upstream's TypeScript object literals.
func jsonMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &m))
	return m
}

func TestSanitizeJSONSchema(t *testing.T) {
	t.Run("strips numeric constraints and adds readable description", func(t *testing.T) {
		input := jsonMap(t, `{
			"type": "object",
			"properties": {
				"recurringIntervalMinutes": {
					"type": "number",
					"exclusiveMinimum": 0,
					"minimum": 1,
					"maximum": 60,
					"exclusiveMaximum": 120
				}
			},
			"required": ["recurringIntervalMinutes"],
			"additionalProperties": false
		}`)

		got := sanitizeJSONSchema(input)

		want := jsonMap(t, `{
			"type": "object",
			"additionalProperties": false,
			"required": ["recurringIntervalMinutes"],
			"properties": {
				"recurringIntervalMinutes": {
					"type": "number",
					"description": "minimum: 1; maximum: 60; exclusive minimum: 0; exclusive maximum: 120."
				}
			}
		}`)
		assert.Equal(t, want, got)
	})

	t.Run("strips string constraints and unsupported formats", func(t *testing.T) {
		input := jsonMap(t, `{
			"type": "object",
			"properties": {
				"slug": {
					"type": "string",
					"description": "A URL slug",
					"minLength": 1,
					"maxLength": 20,
					"pattern": "^[a-z0-9-]+$",
					"format": "regex"
				}
			}
		}`)

		got := sanitizeJSONSchema(input)

		want := jsonMap(t, `{
			"type": "object",
			"additionalProperties": false,
			"properties": {
				"slug": {
					"type": "string",
					"description": "A URL slug\nmin length: 1; max length: 20; pattern: ^[a-z0-9-]+$; format: regex."
				}
			}
		}`)
		assert.Equal(t, want, got)
	})

	t.Run("recurses into arrays, $defs, and composition schemas", func(t *testing.T) {
		input := jsonMap(t, `{
			"type": "object",
			"$defs": {
				"PositiveInteger": {
					"type": "integer",
					"minimum": 1
				}
			},
			"properties": {
				"count": { "$ref": "#/$defs/PositiveInteger" },
				"tags": {
					"type": "array",
					"minItems": 2,
					"maxItems": 4,
					"uniqueItems": true,
					"items": {
						"anyOf": [
							{ "type": "string", "minLength": 1 },
							{ "type": "number", "maximum": 10 }
						]
					}
				}
			}
		}`)

		got := sanitizeJSONSchema(input)

		want := jsonMap(t, `{
			"type": "object",
			"additionalProperties": false,
			"$defs": {
				"PositiveInteger": {
					"type": "integer",
					"description": "minimum: 1."
				}
			},
			"properties": {
				"count": { "$ref": "#/$defs/PositiveInteger" },
				"tags": {
					"type": "array",
					"description": "min items: 2; max items: 4; unique items: true.",
					"items": {
						"anyOf": [
							{ "type": "string", "description": "min length: 1." },
							{ "type": "number", "description": "maximum: 10." }
						]
					}
				}
			}
		}`)
		assert.Equal(t, want, got)
	})

	t.Run("recurses into draft-7 definitions key", func(t *testing.T) {
		input := jsonMap(t, `{
			"definitions": {
				"PositiveInteger": {
					"type": "integer",
					"minimum": 1
				},
				"BoundedString": {
					"type": "string",
					"minLength": 1,
					"maxLength": 10
				}
			}
		}`)

		got := sanitizeJSONSchema(input)

		want := jsonMap(t, `{
			"definitions": {
				"PositiveInteger": {
					"type": "integer",
					"description": "minimum: 1."
				},
				"BoundedString": {
					"type": "string",
					"description": "min length: 1; max length: 10."
				}
			}
		}`)
		assert.Equal(t, want, got)
	})

	t.Run("converts oneOf to anyOf", func(t *testing.T) {
		input := jsonMap(t, `{
			"oneOf": [
				{ "type": "string", "minLength": 1 },
				{ "type": "number", "minimum": 0 }
			]
		}`)

		got := sanitizeJSONSchema(input)

		want := jsonMap(t, `{
			"anyOf": [
				{ "type": "string", "description": "min length: 1." },
				{ "type": "number", "description": "minimum: 0." }
			]
		}`)
		assert.Equal(t, want, got)
	})

	t.Run("$ref short-circuits and drops siblings", func(t *testing.T) {
		input := jsonMap(t, `{
			"$ref": "#/$defs/Foo",
			"minLength": 1,
			"description": "ignored",
			"type": "string"
		}`)

		got := sanitizeJSONSchema(input)

		want := jsonMap(t, `{ "$ref": "#/$defs/Foo" }`)
		assert.Equal(t, want, got)
	})

	t.Run("supported format values are preserved", func(t *testing.T) {
		input := jsonMap(t, `{
			"type": "string",
			"format": "email"
		}`)

		got := sanitizeJSONSchema(input)

		want := jsonMap(t, `{
			"type": "string",
			"format": "email"
		}`)
		assert.Equal(t, want, got)
	})

	t.Run("uniqueItems false is not reported", func(t *testing.T) {
		input := jsonMap(t, `{
			"type": "array",
			"uniqueItems": false,
			"minItems": 1
		}`)

		got := sanitizeJSONSchema(input)

		want := jsonMap(t, `{
			"type": "array",
			"description": "min items: 1."
		}`)
		assert.Equal(t, want, got)
	})

	t.Run("object node without additionalProperties gets it forced to false", func(t *testing.T) {
		input := jsonMap(t, `{
			"type": "object",
			"properties": {
				"name": { "type": "string" }
			}
		}`)

		got := sanitizeJSONSchema(input)

		additional, ok := got["additionalProperties"]
		require.True(t, ok)
		assert.Equal(t, false, additional)
	})

	t.Run("object node with type but no properties still sets additionalProperties: false", func(t *testing.T) {
		input := jsonMap(t, `{ "type": "object" }`)

		got := sanitizeJSONSchema(input)

		want := jsonMap(t, `{
			"type": "object",
			"additionalProperties": false
		}`)
		assert.Equal(t, want, got)
		_, hasProps := got["properties"]
		assert.False(t, hasProps, "no properties on output when absent in input")
	})

	t.Run("non-mutation: input schema is unchanged after sanitization", func(t *testing.T) {
		raw := `{
			"type": "object",
			"properties": {
				"value": { "type": "number", "exclusiveMinimum": 0 }
			}
		}`
		input := jsonMap(t, raw)
		before, err := json.Marshal(input)
		require.NoError(t, err)

		_ = sanitizeJSONSchema(input)

		after, err := json.Marshal(input)
		require.NoError(t, err)
		assert.JSONEq(t, string(before), string(after), "sanitizer must not mutate input")
	})

	t.Run("not keyword is stripped and JSON-encoded into description", func(t *testing.T) {
		input := jsonMap(t, `{
			"type": "object",
			"not": { "type": "null" }
		}`)

		got := sanitizeJSONSchema(input)

		_, hasNot := got["not"]
		assert.False(t, hasNot, "not should be stripped")
		assert.Equal(t, `not: {"type":"null"}.`, got["description"])
	})

	t.Run("multipleOf and integer values render via JSON", func(t *testing.T) {
		input := jsonMap(t, `{
			"type": "number",
			"multipleOf": 5,
			"minimum": 10
		}`)

		got := sanitizeJSONSchema(input)

		want := jsonMap(t, `{
			"type": "number",
			"description": "minimum: 10; multiple of: 5."
		}`)
		assert.Equal(t, want, got)
	})

	t.Run("allOf is preserved (not rewritten)", func(t *testing.T) {
		input := jsonMap(t, `{
			"allOf": [
				{ "type": "object", "properties": { "a": { "type": "string", "minLength": 1 } } }
			]
		}`)

		got := sanitizeJSONSchema(input)

		_, hasAllOf := got["allOf"]
		assert.True(t, hasAllOf, "allOf should remain as allOf")
		_, hasAnyOf := got["anyOf"]
		assert.False(t, hasAnyOf, "no anyOf should be synthesized from allOf")
	})

	t.Run("tuple items (array form) are sanitized element-wise", func(t *testing.T) {
		input := jsonMap(t, `{
			"type": "array",
			"items": [
				{ "type": "string", "minLength": 1 },
				{ "type": "number", "maximum": 5 }
			]
		}`)

		got := sanitizeJSONSchema(input)

		want := jsonMap(t, `{
			"type": "array",
			"items": [
				{ "type": "string", "description": "min length: 1." },
				{ "type": "number", "description": "maximum: 5." }
			]
		}`)
		assert.Equal(t, want, got)
	})

	t.Run("boolean schema definition is passed through", func(t *testing.T) {
		input := jsonMap(t, `{
			"type": "object",
			"properties": {
				"any": true,
				"none": false
			}
		}`)

		got := sanitizeJSONSchema(input)

		props, ok := got["properties"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, props["any"])
		assert.Equal(t, false, props["none"])
	})

	t.Run("preserves $schema, $id, title, enum, const, and default", func(t *testing.T) {
		input := jsonMap(t, `{
			"$schema": "http://json-schema.org/draft-07/schema#",
			"$id": "https://example.com/schema",
			"title": "Color",
			"type": "string",
			"enum": ["red", "green", "blue"],
			"const": "red",
			"default": "red"
		}`)

		got := sanitizeJSONSchema(input)

		assert.Equal(t, "http://json-schema.org/draft-07/schema#", got["$schema"])
		assert.Equal(t, "https://example.com/schema", got["$id"])
		assert.Equal(t, "Color", got["title"])
		assert.Equal(t, "string", got["type"])
		assert.Equal(t, []any{"red", "green", "blue"}, got["enum"])
		assert.Equal(t, "red", got["const"])
		assert.Equal(t, "red", got["default"])
	})

	t.Run("required is preserved on object nodes", func(t *testing.T) {
		input := jsonMap(t, `{
			"type": "object",
			"properties": {
				"name": { "type": "string" }
			},
			"required": ["name"]
		}`)

		got := sanitizeJSONSchema(input)

		assert.Equal(t, []any{"name"}, got["required"])
	})
}

func TestFormatConstraintName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"minimum", "minimum"},
		{"minLength", "min length"},
		{"exclusiveMinimum", "exclusive minimum"},
		{"multipleOf", "multiple of"},
		{"uniqueItems", "unique items"},
		{"maxProperties", "max properties"},
		{"not", "not"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, formatConstraintName(tc.in), tc.in)
	}
}

func TestFormatConstraintValue(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"string verbatim", "^[a-z]+$", "^[a-z]+$"},
		{"integer-as-float64", float64(5), "5"},
		{"float", 1.5, "1.5"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"object", map[string]any{"type": "string"}, `{"type":"string"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, formatConstraintValue(tc.in))
		})
	}
}
