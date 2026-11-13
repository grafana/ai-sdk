package tenanttype

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
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
		Decode: func(json.RawMessage) (any, error) { return "decoded", nil },
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
	decoded, err := definition.Decode(nil)
	require.NoError(t, err)
	assert.Equal(t, "decoded", decoded)
}

// namePattern is the extension name rule: a DNS label.
var namePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// reservedMembers are the Middleware spec fields a member must not shadow.
var reservedMembers = []string{"type", "enabled"}

// problems returns what is wrong with a registry: invalid or repeated names,
// reserved or shared members, examples that don't match the schema, and a
// type enabled by default whose schema refuses the empty configuration.
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
		if tenantType.EnabledByDefault && tenantType.ConfigSchema.Validate(json.RawMessage(`{}`)) != nil {
			found = append(found, fmt.Sprintf("%q is enabled by default but its schema refuses the empty configuration", tenantType.Name))
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
	required, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`))
	require.NoError(t, err)
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
		{name: "default-enabled type whose schema refuses the empty configuration", types: []Type{{Name: "required", EnabledByDefault: true, ConfigSchema: required, Example: json.RawMessage(`{"text":"hi"}`)}}, problem: `"required" is enabled by default but its schema refuses the empty configuration`},
		{name: "example without schema", types: []Type{{Name: "marker", Example: json.RawMessage(`{}`)}}, problem: `"marker" has an example but no schema`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Contains(t, problems(tc.types), tc.problem)
		})
	}
	assert.Empty(t, problems([]Type{instructions, {Name: "marker"}}))
}
