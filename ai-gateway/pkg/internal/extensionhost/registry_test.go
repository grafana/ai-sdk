package extensionhost

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
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
		{name: "duplicate names", definitions: []extension.Definition{validDefinition("quota"), validDefinition("quota")}, message: `extension name "quota" is registered twice`},
		{name: "unknown mandatory", definitions: []extension.Definition{validDefinition("quota")}, mandatory: []string{"audit"}, message: `mandatory extension "audit" is not registered`},
		{name: "mandatory tenant", definitions: with(func(d *extension.Definition) { d.Scope = extension.ScopeTenant }), mandatory: []string{"quota"}, message: `mandatory extension "quota" must have operator scope`},
		{name: "repeated mandatory", definitions: []extension.Definition{validDefinition("quota")}, mandatory: []string{"quota", "quota"}, message: "listed twice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDefinitions(tc.definitions, tc.mandatory)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
		})
	}

	t.Run("valid", func(t *testing.T) {
		customer := validDefinition("audit")
		customer.Keys = []extension.KeyOwnership{extension.KeysCustomer}
		customer.Secrets = []string{"apiKey", "endpointToken"}
		assert.NoError(t, ValidateDefinitions([]extension.Definition{validDefinition("quota"), customer}, []string{"quota"}))
		assert.NoError(t, ValidateDefinitions(nil, nil))
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
