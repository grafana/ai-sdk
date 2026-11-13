package tenanttype

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/schema"
)

func TestType_Member(t *testing.T) {
	for name, member := range map[string]string{
		"marker":               "marker",
		"default-instructions": "defaultInstructions",
		"model-access-2":       "modelAccess2",
		"a-1b":                 "a1b",
		"a-é":                  "aé",
	} {
		assert.Equal(t, member, Type{Name: name}.Member(), name)
	}
}

func TestType_Bind(t *testing.T) {
	configSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"additionalProperties":false}`))
	require.NoError(t, err)
	tenantType := Type{Name: "default-instructions", ConfigSchema: configSchema, EnabledByDefault: true}
	built := false
	definition := tenantType.Bind(Implementation{
		Keys:          []extension.KeyOwnership{extension.KeysCustomer},
		FailurePolicy: extension.FailSkip,
		New: func(json.RawMessage, extension.Dependencies) (any, error) {
			built = true
			return nil, nil
		},
	})
	assert.Equal(t, "default-instructions", definition.Name)
	assert.Equal(t, extension.ScopeTenant, definition.Scope)
	assert.Equal(t, []extension.KeyOwnership{extension.KeysCustomer}, definition.Keys)
	assert.Equal(t, extension.FailSkip, definition.FailurePolicy)
	assert.JSONEq(t, string(configSchema.JSON()), string(definition.ConfigSchema.JSON()))
	assert.True(t, definition.EnabledByDefault)
	assert.Empty(t, definition.Secrets)
	_, err = definition.New(nil, extension.Dependencies{})
	require.NoError(t, err)
	assert.True(t, built)
}

// namePattern is the extension name rule: a DNS label.
var namePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// reservedMembers are the Middleware spec fields a member must not shadow.
var reservedMembers = []string{"type", "enabled"}

// problems returns what is wrong with a registry: invalid or repeated names,
// reserved or shared members, and examples that don't match the schema.
func problems(types []Type) []string {
	var found []string
	names := map[string]bool{}
	members := map[string]string{}
	for _, tenantType := range types {
		if !namePattern.MatchString(tenantType.Name) {
			found = append(found, fmt.Sprintf("%q is not a valid name", tenantType.Name))
		}
		if names[tenantType.Name] {
			found = append(found, fmt.Sprintf("%q is declared twice", tenantType.Name))
		}
		names[tenantType.Name] = true
		if tenantType.ConfigSchema.JSON() == nil {
			if tenantType.Example != nil {
				found = append(found, fmt.Sprintf("%q has an example but no schema", tenantType.Name))
			}
			continue
		}
		member := tenantType.Member()
		if slices.Contains(reservedMembers, member) {
			found = append(found, fmt.Sprintf("%q has the reserved member %q", tenantType.Name, member))
		}
		if other, taken := members[member]; taken {
			found = append(found, fmt.Sprintf("%q and %q share the member %q", other, tenantType.Name, member))
		}
		members[member] = tenantType.Name
		if tenantType.Example == nil || tenantType.ConfigSchema.Validate(tenantType.Example) != nil {
			found = append(found, fmt.Sprintf("%q needs an example its schema accepts", tenantType.Name))
		}
		decoder := json.NewDecoder(bytes.NewReader(tenantType.ConfigSchema.JSON()))
		decoder.UseNumber()
		var root map[string]any
		if decoder.Decode(&root) != nil || root["type"] != "object" {
			found = append(found, fmt.Sprintf("%q: the schema must be an object", tenantType.Name))
			continue
		}
		for _, problem := range schemaProblems(root, "") {
			found = append(found, fmt.Sprintf("%q: %s", tenantType.Name, problem))
		}
	}
	return found
}

// structureKeywords describe a member's structure; the manifest mirrors them.
var structureKeywords = []string{"type", "description", "properties", "required", "items", "additionalProperties"}

// valueKeywords constrain values; admission checks them and the manifest
// leaves them out.
var valueKeywords = []string{
	"enum", "const", "pattern", "format", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf",
	"minLength", "maxLength", "minItems", "maxItems", "minProperties", "maxProperties", "uniqueItems",
	"default", "examples", "title", "$schema", "$id", "$comment",
}

var jsonTypes = []string{"string", "integer", "number", "boolean", "array", "object"}

const maxSafeInteger = 1<<53 - 1

// schemaProblems checks that a schema can be mirrored by a structural
// manifest member: one JSON type per node, closed objects and arrays only,
// no typed maps, references or combinators, and integer bounds within the
// safe integers.
func schemaProblems(node map[string]any, at string) []string {
	where := at
	if where == "" {
		where = "/"
	}
	var found []string
	for keyword := range node {
		if !slices.Contains(structureKeywords, keyword) && !slices.Contains(valueKeywords, keyword) {
			found = append(found, fmt.Sprintf("%s uses %q", where, keyword))
		}
	}
	name, _ := node["type"].(string)
	if !slices.Contains(jsonTypes, name) {
		return append(found, where+" must have exactly one JSON type")
	}
	switch name {
	case "object":
		properties, _ := node["properties"].(map[string]any)
		if closed, ok := node["additionalProperties"].(bool); len(properties) == 0 || !ok || closed {
			return append(found, where+" must declare properties and set additionalProperties to false")
		}
		for property, child := range properties {
			child, _ := child.(map[string]any)
			found = append(found, schemaProblems(child, at+"/properties/"+property)...)
		}
	case "array":
		items, ok := node["items"].(map[string]any)
		if !ok {
			return append(found, where+" must declare items as one schema")
		}
		found = append(found, schemaProblems(items, at+"/items")...)
	case "integer":
		for _, keyword := range []string{"minimum", "maximum"} {
			value, declared := node[keyword]
			if !declared {
				continue
			}
			number, _ := value.(json.Number)
			if bound, err := strconv.ParseInt(number.String(), 10, 64); err != nil || bound < -maxSafeInteger || bound > maxSafeInteger {
				found = append(found, fmt.Sprintf("%s must set %s within ±%d", where, keyword, maxSafeInteger))
			}
		}
	}
	return found
}

func TestAll(t *testing.T) {
	assert.Empty(t, problems(All()))
	assert.Empty(t, All(), "the shipped registry is empty")
	previous := registry
	t.Cleanup(func() { registry = previous })
	registry = []Type{{Name: "b"}, {Name: "a"}}
	types := All()
	assert.Equal(t, []Type{{Name: "a"}, {Name: "b"}}, types)
	types[0].Name = "changed"
	assert.Equal(t, "b", registry[0].Name, "All returns a copy")
}

func TestProblems(t *testing.T) {
	configSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","maxLength":4}},"additionalProperties":false}`))
	require.NoError(t, err)
	instructions := Type{Name: "default-instructions", ConfigSchema: configSchema, Example: json.RawMessage(`{"text":"hi"}`)}
	with := func(change func(*Type)) []Type {
		changed := instructions
		change(&changed)
		return []Type{changed}
	}
	for _, tc := range []struct {
		name    string
		types   []Type
		problem string
	}{
		{name: "invalid name", types: with(func(tt *Type) { tt.Name = "Default" }), problem: `"Default" is not a valid name`},
		{name: "repeated name", types: []Type{instructions, instructions}, problem: `"default-instructions" is declared twice`},
		{name: "reserved member", types: with(func(tt *Type) { tt.Name = "enabled" }), problem: `"enabled" has the reserved member "enabled"`},
		{name: "shared member", types: []Type{{Name: "a-1b", ConfigSchema: configSchema, Example: instructions.Example}, {Name: "a1b", ConfigSchema: configSchema, Example: instructions.Example}}, problem: `"a-1b" and "a1b" share the member "a1b"`},
		{name: "schema without example", types: with(func(tt *Type) { tt.Example = nil }), problem: `"default-instructions" needs an example its schema accepts`},
		{name: "example the schema refuses", types: with(func(tt *Type) { tt.Example = json.RawMessage(`{"text":"too long"}`) }), problem: `"default-instructions" needs an example its schema accepts`},
		{name: "example without schema", types: []Type{{Name: "marker", Example: json.RawMessage(`{}`)}}, problem: `"marker" has an example but no schema`},
		{name: "schema not an object", types: withSchema(t, `{"type":"string"}`), problem: `"shaped": the schema must be an object`},
		{name: "open object", types: withSchema(t, object(`"a":{"type":"object"}`)), problem: `"shaped": /properties/a must declare properties and set additionalProperties to false`},
		{name: "typed map", types: withSchema(t, object(`"a":{"type":"object","additionalProperties":{"type":"string"}}`)), problem: `"shaped": /properties/a must declare properties and set additionalProperties to false`},
		{name: "object without closing", types: withSchema(t, `{"type":"object","properties":{"a":{"type":"string"}}}`), problem: `"shaped": / must declare properties and set additionalProperties to false`},
		{name: "reference", types: withSchema(t, object(`"a":{"$ref":"#/properties/b"},"b":{"type":"string"}`)), problem: `"shaped": /properties/a uses "$ref"`},
		{name: "combinator", types: withSchema(t, object(`"a":{"type":"string","oneOf":[{"type":"string"}]}`)), problem: `"shaped": /properties/a uses "oneOf"`},
		{name: "type array", types: withSchema(t, object(`"a":{"type":["string","null"]}`)), problem: `"shaped": /properties/a must have exactly one JSON type`},
		{name: "array without items", types: withSchema(t, object(`"a":{"type":"array"}`)), problem: `"shaped": /properties/a must declare items as one schema`},
		{name: "nested open object", types: withSchema(t, object(`"a":{"type":"array","items":{"type":"object"}}`)), problem: `"shaped": /properties/a/items must declare properties and set additionalProperties to false`},
		{name: "integer bound beyond the safe range", types: withSchema(t, object(`"a":{"type":"integer","maximum":9007199254740992}`)), problem: `"shaped": /properties/a must set maximum within ±9007199254740991`},
		{name: "fractional integer bound", types: withSchema(t, object(`"a":{"type":"integer","minimum":0.5}`)), problem: `"shaped": /properties/a must set minimum within ±9007199254740991`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Contains(t, problems(tc.types), tc.problem)
		})
	}
	assert.Empty(t, problems([]Type{instructions, {Name: "marker"}}))
	assert.Empty(t, problems(withSchema(t, `{"type":"object","description":"d","properties":{"rules":{"type":"array","maxItems":3,"items":{"type":"object","properties":{"models":{"type":"array","items":{"type":"string","pattern":"^[a-z]+$"}},"rpm":{"type":"integer","minimum":1,"maximum":9007199254740991},"temperature":{"type":"number"},"strict":{"type":"boolean"}},"required":["models"],"additionalProperties":false}}},"additionalProperties":false}`)))
}

func object(properties string) string {
	return `{"type":"object","properties":{` + properties + `},"additionalProperties":false}`
}

// withSchema returns one type with configSchema and an empty example, which
// every schema in these tests accepts.
func withSchema(t *testing.T, configSchema string) []Type {
	t.Helper()
	compiled, err := schema.SchemaFromJSON(json.RawMessage(configSchema))
	require.NoError(t, err)
	return []Type{{Name: "shaped", ConfigSchema: compiled, Example: json.RawMessage(`{}`)}}
}
