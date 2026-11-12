package extensionhost

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/schema"
)

func validDefinition(name string) extension.Definition {
	return extension.Definition{
		Name:          name,
		Scope:         extension.ScopeOperator,
		FailurePolicy: extension.FailClosed,
		New:           func(json.RawMessage, extension.Dependencies) (any, error) { return &recorder{}, nil },
	}
}

func TestValidateDefinitions(t *testing.T) {
	with := func(change func(*extension.Definition)) []extension.Definition {
		definition := validDefinition("quota")
		change(&definition)
		return []extension.Definition{definition}
	}
	for _, tc := range []struct {
		name        string
		definitions []extension.Definition
		mandatory   []string
		message     string
	}{
		{name: "empty name", definitions: with(func(d *extension.Definition) { d.Name = "" }), message: `extension name ""`},
		{name: "uppercase name", definitions: with(func(d *extension.Definition) { d.Name = "Quota" }), message: `extension name "Quota"`},
		{name: "leading hyphen", definitions: with(func(d *extension.Definition) { d.Name = "-quota" }), message: `extension name "-quota"`},
		{name: "name too long", definitions: with(func(d *extension.Definition) { d.Name = string(make([]byte, 64)) }), message: "extension name"},
		{name: "unknown scope", definitions: with(func(d *extension.Definition) { d.Scope = "global" }), message: `extension "quota" has an invalid scope`},
		{name: "missing failure policy", definitions: with(func(d *extension.Definition) { d.FailurePolicy = "" }), message: `extension "quota" has an invalid failure policy`},
		{name: "missing factory", definitions: with(func(d *extension.Definition) { d.New = nil }), message: `extension "quota" has an invalid factory`},
		{name: "unknown key ownership", definitions: with(func(d *extension.Definition) { d.Keys = []extension.KeyOwnership{"tenant"} }), message: "invalid key ownership"},
		{name: "repeated key ownership", definitions: with(func(d *extension.Definition) {
			d.Keys = []extension.KeyOwnership{extension.KeysOperator, extension.KeysOperator}
		}), message: "invalid key ownership"},
		{name: "invalid secret name", definitions: with(func(d *extension.Definition) { d.Secrets = []string{"api_key"} }), message: "invalid secret name"},
		{name: "repeated secret", definitions: with(func(d *extension.Definition) { d.Secrets = []string{"apiKey", "apiKey"} }), message: "invalid secret name"},
		{name: "operator decoder", definitions: with(func(d *extension.Definition) { d.Decode = func(json.RawMessage) (any, error) { return nil, nil } }), message: `operator extension "quota" declares a decoder, which only tenant extensions take`},
		{name: "tenant secrets", definitions: with(func(d *extension.Definition) { d.Scope, d.Secrets = extension.ScopeTenant, []string{"apiKey"} }), message: `tenant extension "quota" declares secrets, which only the operator setting supplies`},
		{name: "duplicate names", definitions: []extension.Definition{validDefinition("quota"), validDefinition("quota")}, message: `extension name "quota" is registered twice`},
		{name: "unknown mandatory", definitions: []extension.Definition{validDefinition("quota")}, mandatory: []string{"audit"}, message: `mandatory extension "audit" is not registered`},
		{name: "mandatory tenant", definitions: with(func(d *extension.Definition) { d.Scope = extension.ScopeTenant }), mandatory: []string{"quota"}, message: `mandatory extension "quota" must have operator scope`},
		{name: "repeated mandatory", definitions: []extension.Definition{validDefinition("quota")}, mandatory: []string{"quota", "quota"}, message: "listed twice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDefinitions(tc.definitions, tc.mandatory, typesOf(tc.definitions))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
		})
	}

	t.Run("valid", func(t *testing.T) {
		customer := validDefinition("audit")
		customer.Keys = []extension.KeyOwnership{extension.KeysCustomer}
		customer.Secrets = []string{"apiKey", "endpointToken"}
		assert.NoError(t, ValidateDefinitions([]extension.Definition{validDefinition("quota"), customer}, []string{"quota"}, nil))
		assert.NoError(t, ValidateDefinitions(nil, nil, nil))
	})
}

func TestValidateDefinitions_TenantTypes(t *testing.T) {
	configSchema := func(t *testing.T, raw string) schema.Schema {
		t.Helper()
		compiled, err := schema.SchemaFromJSON(json.RawMessage(raw))
		require.NoError(t, err)
		return compiled
	}
	instructionsSchema := configSchema(t, `{"type":"object","properties":{"text":{"type":"string","maxLength":4}},"additionalProperties":false}`)
	instructions := tenanttype.Type{Name: "default-instructions", ConfigSchema: instructionsSchema, Example: json.RawMessage(`{"text":"hi"}`)}
	marker := tenanttype.Type{Name: "marker", EnabledByDefault: true}
	bound := func(tenantType tenanttype.Type, change ...func(*extension.Definition)) []extension.Definition {
		definition := tenantType.Bind(tenanttype.Implementation{FailurePolicy: extension.FailSkip, New: validDefinition("x").New})
		for _, apply := range change {
			apply(&definition)
		}
		return []extension.Definition{definition}
	}
	withType := func(change func(*tenanttype.Type)) []tenanttype.Type {
		changed := instructions
		change(&changed)
		return []tenanttype.Type{changed}
	}
	for _, tc := range []struct {
		name        string
		definitions []extension.Definition
		types       []tenanttype.Type
		message     string
	}{
		{name: "invalid type name", types: withType(func(tt *tenanttype.Type) { tt.Name = "Default" }), message: `tenant type name "Default"`},
		{name: "repeated type", types: []tenanttype.Type{instructions, instructions}, message: `tenant type "default-instructions" is declared twice`},
		{name: "reserved member", types: withType(func(tt *tenanttype.Type) { tt.Name = "enabled" }), message: `tenant type "enabled" has the reserved member "enabled"`},
		{name: "shared member", types: []tenanttype.Type{{Name: "a-1b", ConfigSchema: instructionsSchema, Example: instructions.Example}, {Name: "a1b", ConfigSchema: instructionsSchema, Example: instructions.Example}}, message: `tenant types "a-1b" and "a1b" share the member "a1b"`},
		{name: "schema without example", types: withType(func(tt *tenanttype.Type) { tt.Example = nil }), message: `tenant type "default-instructions" needs an example`},
		{name: "example the schema refuses", types: withType(func(tt *tenanttype.Type) { tt.Example = json.RawMessage(`{"text":"too long"}`) }), message: `tenant type "default-instructions" needs an example`},
		{name: "example over a bound above 2^53", types: []tenanttype.Type{{Name: "bounded", ConfigSchema: configSchema(t, `{"type":"object","properties":{"n":{"type":"integer","maximum":9007199254740992}},"additionalProperties":false}`), Example: json.RawMessage(`{"n":9007199254740993}`)}}, message: `tenant type "bounded" needs an example`},
		{name: "example without schema", types: []tenanttype.Type{{Name: "marker", Example: json.RawMessage(`{}`)}}, message: `tenant type "marker" has an example but no configuration schema`},
		{name: "tenant definition without a type", definitions: bound(instructions), message: `tenant extension "default-instructions" has no public tenant type`},
		{name: "different schema", definitions: bound(instructions, func(d *extension.Definition) {
			d.ConfigSchema = configSchema(t, `{"type":"object","properties":{"text":{"type":"string"}},"additionalProperties":false}`)
		}), types: []tenanttype.Type{instructions}, message: `tenant extension "default-instructions" differs from its public tenant type in its configuration schema`},
		{name: "schema on a type without one", definitions: bound(marker, func(d *extension.Definition) { d.ConfigSchema = instructionsSchema }), types: []tenanttype.Type{marker}, message: "in its configuration schema"},
		{name: "different default", definitions: bound(marker, func(d *extension.Definition) { d.EnabledByDefault = false }), types: []tenanttype.Type{marker}, message: `tenant extension "marker" differs from its public tenant type in its default state`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDefinitions(tc.definitions, nil, tc.types)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
		})
	}

	t.Run("valid", func(t *testing.T) {
		operator := validDefinition("default-instructions")
		assert.NoError(t, ValidateDefinitions(append(bound(marker), bound(instructions)...), nil, []tenanttype.Type{instructions, marker}))
		assert.NoError(t, ValidateDefinitions([]extension.Definition{operator}, nil, []tenanttype.Type{instructions}), "an operator definition may share a type's name")
		assert.NoError(t, ValidateDefinitions(nil, nil, []tenanttype.Type{instructions}), "a type may have no implementation")
	})
}

func TestApplies(t *testing.T) {
	everywhere := validDefinition("all")
	operator := validDefinition("operator")
	operator.Keys = []extension.KeyOwnership{extension.KeysOperator}
	customer := validDefinition("customer")
	customer.Keys = []extension.KeyOwnership{extension.KeysCustomer}
	both := validDefinition("both")
	both.Keys = []extension.KeyOwnership{extension.KeysOperator, extension.KeysCustomer}
	for _, tc := range []struct {
		definition extension.Definition
		operator   bool
		customer   bool
	}{
		{definition: everywhere, operator: true, customer: true},
		{definition: operator, operator: true},
		{definition: customer, customer: true},
		{definition: both, operator: true, customer: true},
	} {
		assert.Equal(t, tc.operator, applies(tc.definition, extension.KeysOperator), tc.definition.Name)
		assert.Equal(t, tc.customer, applies(tc.definition, extension.KeysCustomer), tc.definition.Name)
	}
}
