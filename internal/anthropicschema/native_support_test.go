package anthropicschema

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeSchema(t *testing.T, raw string) map[string]any {
	t.Helper()
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &schema))
	return schema
}

func stringProperties(count int, prefix string) string {
	fields := make([]string, count)
	for i := range fields {
		fields[i] = fmt.Sprintf(`%q:{"type":"string"}`, fmt.Sprintf("%s%d", prefix, i))
	}
	return "{" + strings.Join(fields, ",") + "}"
}

func optionalObject(count int, prefix string) string {
	return fmt.Sprintf(`{"type":"object","properties":%s,"additionalProperties":false}`, stringProperties(count, prefix))
}

func requiredNames(count int, prefix string) string {
	names := make([]string, count)
	for i := range names {
		names[i] = fmt.Sprintf("%q", fmt.Sprintf("%s%d", prefix, i))
	}
	return "[" + strings.Join(names, ",") + "]"
}

func wrapField(field string) string {
	return fmt.Sprintf(`{"type":"object","properties":{"field":%s},"required":["field"],"additionalProperties":false}`, field)
}

func TestSupportsNativeStructuredOutput(t *testing.T) {
	referencedTwice := func(count int) string {
		return fmt.Sprintf(`{"type":"object","properties":{"x":{"$ref":"#/$defs/D"},"y":{"$ref":"#/$defs/D"}},"required":["x","y"],"additionalProperties":false,"$defs":{"D":%s}}`, optionalObject(count, "d"))
	}

	tests := []struct {
		name   string
		schema string
		want   bool
	}{
		{"closed object", `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`, true},
		{"object without additionalProperties that has properties", `{"type":"object","properties":{"answer":{"type":"string"}}}`, true},
		{"many required properties", fmt.Sprintf(`{"type":"object","properties":%s,"required":%s,"additionalProperties":false}`, stringProperties(60, "f"), requiredNames(60, "f")), true},
		{"self-referencing schema", `{"type":"object","properties":{"root":{"$ref":"#/$defs/node"}},"$defs":{"node":{"type":"object","properties":{"children":{"items":{"$ref":"#/$defs/node"}}}}}}`, false},
		{"map", wrapField(`{"type":"object","additionalProperties":{"type":"number"}}`), false},
		{"additionalProperties true", wrapField(`{"type":"object","additionalProperties":true}`), false},
		{"free-form object", wrapField(`{"type":"object"}`), false},
		{"patternProperties", wrapField(`{"type":"object","patternProperties":{"^x-":{"type":"string"}}}`), false},
		{"open object reached through a $ref", `{"type":"object","properties":{"scores":{"$ref":"#/$defs/Scores"}},"required":["scores"],"additionalProperties":false,"$defs":{"Scores":{"type":"object","additionalProperties":{"type":"number"}}}}`, false},
		{"empty closed object", `{"type":"object","additionalProperties":false}`, true},
		{"10 optional properties", optionalObject(10, "f"), true},
		{"11 optional properties", optionalObject(11, "f"), false},
		{"optional properties across nested objects", fmt.Sprintf(`{"type":"object","properties":{"a":{"type":"string"},"nested":%s},"required":["nested"],"additionalProperties":false}`, optionalObject(10, "f")), false},
		{"optional properties in array items and union branches", fmt.Sprintf(`{"type":"object","properties":{"items":{"type":"array","items":%s},"union":{"anyOf":[%s,%s]}},"required":["items","union"],"additionalProperties":false}`, optionalObject(5, "i"), optionalObject(3, "a"), optionalObject(3, "b")), false},
		{"referenced definition counted at every use within budget", referencedTwice(5), true},
		{"referenced definition counted at every use over budget", referencedTwice(6), false},
		{"unreferenced definitions ignored", fmt.Sprintf(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false,"$defs":{"Unused":%s}}`, optionalObject(30, "u")), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, SupportsNativeStructuredOutput(decodeSchema(t, tc.schema)))
		})
	}
}

func TestHasCircularReference(t *testing.T) {
	tests := []struct {
		name   string
		schema string
		want   bool
	}{
		{"no references", `{"type":"object","properties":{"name":{"type":"string"}}}`, false},
		{"shared definitions without a cycle", `{"type":"object","properties":{"home":{"$ref":"#/$defs/address"},"work":{"$ref":"#/$defs/address"}},"$defs":{"address":{"type":"object","properties":{"country":{"$ref":"#/$defs/country"}}},"country":{"type":"string"}}}`, false},
		{"top-level $ref to a non-recursive definition", `{"$ref":"#/definitions/Answer","definitions":{"Answer":{"type":"object","properties":{"answer":{"type":"string"}}}}}`, false},
		{"self-referencing definition", `{"type":"object","properties":{"root":{"$ref":"#/$defs/node"}},"$defs":{"node":{"type":"object","properties":{"children":{"type":"array","items":{"$ref":"#/$defs/node"}}}}}}`, true},
		{"mutually referencing definitions", `{"$ref":"#/definitions/a","definitions":{"a":{"type":"object","properties":{"b":{"$ref":"#/definitions/b"}}},"b":{"type":"object","properties":{"a":{"$ref":"#/definitions/a"}}}}}`, true},
		{"reference to the root schema", `{"type":"object","properties":{"parent":{"anyOf":[{"$ref":"#"},{"type":"null"}]}}}`, true},
		{"cycles in unreferenced definitions", `{"type":"object","properties":{"name":{"type":"string"}},"$defs":{"node":{"items":{"$ref":"#/$defs/node"}}}}`, false},
		{"properties named like keywords are subschemas", `{"type":"object","properties":{"definitions":{"$ref":"#/$defs/node"},"$ref":{"type":"string"}},"$defs":{"node":{"type":"object","properties":{"next":{"$ref":"#/$defs/node"}}}}}`, true},
		{"$ref keys inside instance data", `{"type":"object","default":{"$ref":"#"},"properties":{"kind":{"enum":[{"$ref":"#"}]}}}`, false},
		{"unresolvable or remote references", `{"type":"object","properties":{"a":{"$ref":"#/$defs/missing"},"b":{"$ref":"https://example.com/schema.json"},"c":{"$ref":"#/%E0%A4%A"}}}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, HasCircularReference(decodeSchema(t, tc.schema)))
		})
	}
}
