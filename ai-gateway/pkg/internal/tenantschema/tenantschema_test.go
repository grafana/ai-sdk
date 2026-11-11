package tenantschema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/schema"
)

const modelAccessSchema = `{
	"type": "object",
	"description": "ignored for the member",
	"properties": {
		"rules": {
			"type": "array",
			"minItems": 1,
			"items": {
				"type": "object",
				"properties": {
					"models": {"type": "array", "items": {"type": "string", "pattern": "^[a-z]+$"}},
					"teams": {"type": "array", "items": {"type": "string"}},
					"limits": {"$ref": "#/$defs/limits"}
				},
				"required": ["models", "teams"],
				"additionalProperties": false
			}
		},
		"perModel": {"type": "object", "additionalProperties": {"type": "object", "properties": {"maxTokens": {"type": "integer", "minimum": 1}}, "additionalProperties": false}},
		"mode": {"type": "string", "enum": ["allow", "deny"], "default": "allow", "description": "Whether rules allow or deny.\nApplies to every rule."},
		"temperature": {"type": "number", "maximum": 2},
		"strict": {"type": "boolean"}
	},
	"required": ["rules"],
	"additionalProperties": false,
	"$defs": {"limits": {"type": "object", "properties": {"rpm": {"type": "integer"}}, "additionalProperties": false}}
}`

const modelAccessExample = `{"rules":[{"models":["balanced"],"teams":["platform"],"limits":{"rpm":10}}],"perModel":{"balanced":{"maxTokens":9007199254740993}},"mode":"deny","temperature":0.5,"strict":true}`

func tenantType(t *testing.T, name, configSchema, example string) tenanttype.Type {
	t.Helper()
	tt := tenanttype.Type{Name: name, Description: "Controls which models teams may use."}
	if configSchema != "" {
		compiled, err := schema.SchemaFromJSON(json.RawMessage(configSchema))
		require.NoError(t, err)
		tt.ConfigSchema = compiled
		tt.Example = json.RawMessage(example)
	}
	return tt
}

func TestProject(t *testing.T) {
	member, ok, err := Project(tenantType(t, "model-access", modelAccessSchema, modelAccessExample))
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "modelAccess", member.Name)
	got, err := json.Marshal(member.Schema.JSONSchema())
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"type": "object",
		"description": "ignored for the member",
		"properties": {
			"rules": {"type": "array", "items": {
				"type": "object",
				"properties": {
					"models": {"type": "array", "items": {"type": "string"}},
					"teams": {"type": "array", "items": {"type": "string"}},
					"limits": {"type": "object", "properties": {"rpm": {"type": "integer"}}, "additionalProperties": false}
				},
				"required": ["models", "teams"],
				"additionalProperties": false
			}},
			"perModel": {"type": "object", "additionalProperties": {"type": "object", "properties": {"maxTokens": {"type": "integer"}}, "additionalProperties": false}},
			"mode": {"type": "string", "description": "Whether rules allow or deny.\nApplies to every rule."},
			"temperature": {"type": "number"},
			"strict": {"type": "boolean"}
		},
		"required": ["rules"],
		"additionalProperties": false
	}`, string(got), "value constraints, defaults and definitions are dropped; references are resolved")

	_, ok, err = Project(tenanttype.Type{Name: "marker"})
	require.NoError(t, err)
	assert.False(t, ok, "a type without configuration has no member")
}

func TestProject_Refuses(t *testing.T) {
	object := func(properties string) string {
		return `{"type":"object","properties":` + properties + `,"additionalProperties":false}`
	}
	for _, tc := range []struct {
		name    string
		schema  string
		message string
	}{
		{name: "root not an object", schema: `{"type":"string"}`, message: "must be an object"},
		{name: "missing type", schema: object(`{"a":{}}`), message: "at /properties/a must have exactly one type"},
		{name: "type array", schema: object(`{"a":{"type":["string","null"]}}`), message: "at /properties/a must have exactly one type"},
		{name: "null type", schema: object(`{"a":{"type":"null"}}`), message: "at /properties/a must have exactly one type"},
		{name: "oneOf", schema: object(`{"a":{"type":"string","oneOf":[{"type":"string"}]}}`), message: `at /properties/a uses "oneOf"`},
		{name: "anyOf", schema: object(`{"a":{"anyOf":[{"type":"string"}]}}`), message: `uses "anyOf"`},
		{name: "patternProperties", schema: object(`{"a":{"type":"object","patternProperties":{"x":{"type":"string"}}}}`), message: `uses "patternProperties"`},
		{name: "open object", schema: object(`{"a":{"type":"object"}}`), message: "at /properties/a must declare properties, or additionalProperties as a schema"},
		{name: "additionalProperties true", schema: object(`{"a":{"type":"object","additionalProperties":true}}`), message: "at /properties/a must declare properties, or additionalProperties as a schema"},
		{name: "empty object", schema: object(`{"a":{"type":"object","properties":{},"additionalProperties":false}}`), message: "at /properties/a must declare properties"},
		{name: "empty root", schema: `{"type":"object","properties":{},"additionalProperties":false}`, message: "at / must declare properties"},
		{name: "properties without closing", schema: `{"type":"object","properties":{"a":{"type":"string"}}}`, message: "at / must set additionalProperties to false"},
		{name: "properties with a typed map", schema: `{"type":"object","properties":{"a":{"type":"string"}},"additionalProperties":{"type":"string"}}`, message: "at / must set additionalProperties to false"},
		{name: "array without items", schema: object(`{"a":{"type":"array"}}`), message: "at /properties/a must declare items as one schema"},
		{name: "required undeclared field", schema: `{"type":"object","properties":{"a":{"type":"string"}},"required":["b"],"additionalProperties":false}`, message: "requires a field it doesn't declare"},
		{name: "properties on a scalar", schema: object(`{"a":{"type":"string","properties":{}}}`), message: `declares "properties" on a string`},
		{name: "reference outside definitions", schema: object(`{"a":{"$ref":"#/properties/b"},"b":{"type":"string"}}`), message: "isn't a local definition"},
		{name: "reference with siblings", schema: `{"type":"object","properties":{"a":{"$ref":"#/$defs/x","type":"object"}},"additionalProperties":false,"$defs":{"x":{"type":"string"}}}`, message: "must use $ref alone"},
		{name: "recursive reference", schema: `{"type":"object","properties":{"a":{"$ref":"#/$defs/node"}},"additionalProperties":false,"$defs":{"node":{"type":"object","properties":{"child":{"$ref":"#/$defs/node"}},"additionalProperties":false}}}`, message: "recursively"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := Project(tenantType(t, "broken", tc.schema, `{}`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), `tenant type "broken"`)
			assert.Contains(t, err.Error(), tc.message)
		})
	}
}

func TestMembers_RoundTrip(t *testing.T) {
	members, err := Members([]tenanttype.Type{tenantType(t, "model-access", modelAccessSchema, modelAccessExample), {Name: "marker"}})
	require.NoError(t, err)
	require.Len(t, members, 1, "a type without configuration has no member")

	s, err := Structural(members)
	require.NoError(t, err)
	require.NoError(t, RoundTrip(s, "model-access", "modelAccess", json.RawMessage(modelAccessExample)))
	err = RoundTrip(s, "model-access", "modelAccess", json.RawMessage(`{"rules":[],"unknown":{"nested":1}}`))
	require.Error(t, err, "pruning empties an undeclared object")
	assert.NotContains(t, err.Error(), "nested")

	open, err := structural([]byte(`{"type":"object","properties":{"spec":{"type":"object","properties":{"config":{"type":"object","additionalProperties":true}},"additionalProperties":false}}}`))
	require.NoError(t, err)
	assert.Error(t, RoundTrip(open, "open", "config", json.RawMessage(`{"nested":{"k":"v"}}`)), "an open object loses nested content")
}

func TestMembers_RefusesAnExampleThePlatformChanges(t *testing.T) {
	_, err := Members([]tenanttype.Type{tenantType(t, "model-access", modelAccessSchema, `{"rules":[],"extra":{"k":"v"}}`)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `tenant type "model-access": the platform's pruning changes the example`)
}

func TestRender(t *testing.T) {
	empty, err := Render(nil)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(empty), "middlewareMembers: {}\n"))

	file, err := Render([]tenanttype.Type{tenantType(t, "model-access", modelAccessSchema, modelAccessExample), {Name: "marker"}})
	require.NoError(t, err)
	assert.Equal(t, membersHeader+`middlewareMembers: {
	// Controls which models teams may use.
	"modelAccess"?: {
		// Whether rules allow or deny.
		// Applies to every rule.
		"mode"?: string
		"perModel"?: {[string]: {
			"maxTokens"?: int
		}}
		"rules": [...{
			"limits"?: {
				"rpm"?: int
			}
			"models": [...string]
			"teams": [...string]
		}]
		"strict"?: bool
		"temperature"?: number
	}
}
`, string(file))
}

func TestMembersFile_MatchesTheRegistry(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join("..", "..", "..", MembersFile))
	require.NoError(t, err)
	rendered, err := Render(tenanttype.All())
	require.NoError(t, err)
	assert.Equal(t, string(rendered), string(committed), "run mise run generate-ai-gateway-kinds")
}
