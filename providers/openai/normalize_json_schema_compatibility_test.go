package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIJSONSchema_Lookaround(t *testing.T) {
	for _, tc := range []struct {
		name, pattern string
		removed       bool
	}{
		{"positive lookahead", `value(?=suffix)`, true},
		{"negative lookahead", `^(?!\.)(?!.*\.\.).+@.+$`, true},
		{"positive lookbehind", `(?<=prefix)value`, true},
		{"negative lookbehind", `(?<!prefix)value`, true},
		{"escaped literal", `\(\?=literal\)`, false},
		{"escaped opening parenthesis", `\(?=literal`, false},
		{"escaped backslash before lookahead", `\\(?=suffix)`, true},
		{"character class", `[(?=!)]`, false},
		{"lookbehind in character class", `[(?<!)]`, false},
		{"lookahead after character class", `[a](?=suffix)`, true},
		{"escaped class delimiter", `[\](?=)]`, false},
		{"noncapturing group", `(?:value)`, false},
		{"named group", `(?<name>value)`, false},
		{"ordinary", `^@[a-zA-Z0-9_]+$`, false},
		{"incomplete group", `(?<`, false},
		{"empty", ``, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"type": "string", "pattern": tc.pattern})
			require.NoError(t, err)
			original := append([]byte(nil), raw...)
			got, warnings, err := normalizeOpenAIJSONSchema(raw)
			require.NoError(t, err)
			assert.Equal(t, original, raw)
			assert.Equal(t, "string", got["type"])
			if tc.removed {
				assert.NotContains(t, got, "pattern")
				assert.Equal(t, []provider.Warning{{
					Type: provider.WarnCompatibility, Feature: "JSON Schema pattern with regex lookaround",
					Details: "OpenAI does not support regex lookaround in JSON Schema patterns. The pattern was removed before sending the schema, so OpenAI will not enforce that constraint.",
				}}, warnings)
			} else {
				assert.Equal(t, tc.pattern, got["pattern"])
				assert.Empty(t, warnings)
			}
		})
	}
}

func TestNormalizeOpenAIJSONSchema_RecursiveLookaround(t *testing.T) {
	raw := json.RawMessage(`{
		"pattern":"(?=root)", "propertyNames":{"type":"string","pattern":"(?=ignored)"},
		"properties":{"a":{"pattern":"(?=a)"},"flag":false},
		"patternProperties":{"(?=key)":{"pattern":"(?<=value)"}},
		"additionalProperties":{"pattern":"(?!a)"}, "additionalItems":{"pattern":"(?<!a)"},
		"items":[true,{"pattern":"(?=a)"}], "contains":{"pattern":"(?=a)"},
		"not":{"pattern":"(?=a)"}, "allOf":[{"pattern":"(?=a)"}],
		"anyOf":[{"pattern":"(?=a)"}], "oneOf":[{"pattern":"(?=a)"}],
		"definitions":{"a":{"pattern":"(?=a)"}}, "$defs":{"a":{"pattern":"(?=a)"}},
		"dependencies":{"a":["b"],"b":{"pattern":"(?=a)"}},
		"if":{"pattern":"(?=a)"}, "then":{"pattern":"(?=a)"}, "else":{"pattern":"(?=a)"},
		"examples":[{"pattern":"(?=untouched)"}], "default":{"pattern":"(?=untouched)"}
	}`)
	original := append(json.RawMessage(nil), raw...)
	got, warnings, err := normalizeOpenAIJSONSchema(raw)
	require.NoError(t, err)
	assert.Equal(t, original, raw)
	assert.Equal(t, []provider.Warning{
		{Type: provider.WarnCompatibility, Feature: "JSON Schema propertyNames", Details: propertyNamesWarningDetails},
		{Type: provider.WarnCompatibility, Feature: "JSON Schema pattern with regex lookaround", Details: "OpenAI does not support regex lookaround in JSON Schema patterns. The pattern was removed before sending the schema, so OpenAI will not enforce that constraint."},
	}, warnings)
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"properties":{"a":{},"flag":false}, "patternProperties":{"(?=key)":{}},
		"additionalProperties":{}, "additionalItems":{}, "items":[true,{}], "contains":{},
		"not":{}, "allOf":[{}], "anyOf":[{}], "oneOf":[{}], "definitions":{"a":{}},
		"$defs":{"a":{}}, "dependencies":{"a":["b"],"b":{}},
		"if":{}, "then":{}, "else":{},
		"examples":[{"pattern":"(?=untouched)"}], "default":{"pattern":"(?=untouched)"}
	}`, string(encoded))
}

func TestNormalizeOpenAIJSONSchema_SingletonReferences(t *testing.T) {
	for _, tc := range []struct {
		name, schema, want string
	}{
		{
			"nested recursive reference with siblings",
			`{"properties":{"relatives":{"items":{"allOf":[{"$ref":"#/definitions/person"}],"description":"relative"}}},"definitions":{"person":{"type":"object"}}}`,
			`{"properties":{"relatives":{"items":{"$ref":"#/definitions/person","description":"relative"}}},"definitions":{"person":{"type":"object"}}}`,
		},
		{
			"root definitions with recursive reference",
			`{"default":{"name":"John"},"allOf":[{"$ref":"#/definitions/person"}],"definitions":{"person":{"type":"object","properties":{"relatives":{"items":{"allOf":[{"$ref":"#/definitions/person"}]}}},"required":["relatives"],"additionalProperties":false}}}`,
			`{"default":{"name":"John"},"type":"object","properties":{"relatives":{"items":{"$ref":"#/definitions/person"}}},"required":["relatives"],"additionalProperties":false,"definitions":{"person":{"type":"object","properties":{"relatives":{"items":{"$ref":"#/definitions/person"}}},"required":["relatives"],"additionalProperties":false}}}`,
		},
		{
			"root defs escaped pointer name and root precedence",
			`{"allOf":[{"$ref":"#/$defs/a~1b~0c~01"}],"description":"root","$defs":{"a/b~c~1":{"type":"object","description":"definition","additionalProperties":false}}}`,
			`{"type":"object","description":"root","additionalProperties":false,"$defs":{"a/b~c~1":{"type":"object","description":"definition","additionalProperties":false}}}`,
		},
		{
			"nested unresolved and external references",
			`{"properties":{"a":{"allOf":[{"$ref":"#/missing"}]},"b":{"allOf":[{"$ref":"https://example.com/schema"}]}}}`,
			`{"properties":{"a":{"$ref":"#/missing"},"b":{"$ref":"https://example.com/schema"}}}`,
		},
		{
			"normalized nested allOf reference",
			`{"properties":{"a":{"allOf":[{"allOf":[{"$ref":"#/$defs/a"}]}]}}}`,
			`{"properties":{"a":{"$ref":"#/$defs/a"}}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(tc.schema)
			got, warnings, err := normalizeOpenAIJSONSchema(raw)
			require.NoError(t, err)
			assert.Empty(t, warnings)
			assert.Equal(t, tc.schema, string(raw))
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
}

func TestNormalizeOpenAIJSONSchema_PreservesOtherReferences(t *testing.T) {
	for _, schema := range []string{
		`{"allOf":[{"$ref":"#/definitions/missing"}],"definitions":{}}`,
		`{"allOf":[{"$ref":"#/$defs/missing"}],"$defs":{}}`,
		`{"allOf":[{"$ref":"https://example.com/schema"}]}`,
		`{"allOf":[{"$ref":"#/properties/person"}],"properties":{"person":{"type":"object"}}}`,
		`{"allOf":[{"$ref":"#/$defs/flag"}],"$defs":{"flag":false}}`,
		`{"allOf":[{"$ref":"#/$defs/"}],"$defs":{"":{"type":"object"}}}`,
		`{"$ref":"#/$defs/person","$defs":{"person":{"type":"object"}}}`,
		`{"properties":{"a":{"allOf":[{"$ref":"#/a"},{"type":"object"}]}}}`,
		`{"properties":{"a":{"allOf":[{"$ref":"#/a","description":"not just a reference"}]}}}`,
		`{"properties":{"a":{"allOf":[false]},"b":{"allOf":[]},"c":{"allOf":[{"$ref":123}]}}}`,
	} {
		t.Run(schema, func(t *testing.T) {
			got, warnings, err := normalizeOpenAIJSONSchema(json.RawMessage(schema))
			require.NoError(t, err)
			assert.Empty(t, warnings)
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, schema, string(encoded))
		})
	}
}

func TestNormalizeOpenAIJSONSchema_RemovedPropertyNamesDoesNotWarnForPattern(t *testing.T) {
	got, warnings, err := normalizeOpenAIJSONSchema(json.RawMessage(`{"propertyNames":{"type":"string","pattern":"(?=ignored)"}}`))
	require.NoError(t, err)
	assert.Empty(t, got)
	require.Len(t, warnings, 1)
	assert.Equal(t, "JSON Schema propertyNames", warnings[0].Feature)
}
