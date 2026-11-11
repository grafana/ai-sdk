package tenanttype

import (
	"encoding/json"
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
		Sources:       []extension.Source{extension.SourceRequestBYOK},
		Priority:      3,
		FailurePolicy: extension.FailSkip,
		New: func(json.RawMessage, extension.Dependencies) (any, error) {
			built = true
			return nil, nil
		},
	})
	assert.Equal(t, "default-instructions", definition.Name)
	assert.Equal(t, extension.ScopeTenant, definition.Scope)
	assert.Equal(t, []extension.KeyOwnership{extension.KeysCustomer}, definition.Keys)
	assert.Equal(t, []extension.Source{extension.SourceRequestBYOK}, definition.Sources)
	assert.Equal(t, 3, definition.Priority)
	assert.Equal(t, extension.FailSkip, definition.FailurePolicy)
	assert.JSONEq(t, string(configSchema.JSON()), string(definition.ConfigSchema.JSON()))
	assert.True(t, definition.EnabledByDefault)
	assert.Empty(t, definition.Secrets)
	_, err = definition.New(nil, extension.Dependencies{})
	require.NoError(t, err)
	assert.True(t, built)
}

func TestAll(t *testing.T) {
	assert.Empty(t, All(), "the shipped registry is empty")
	previous := registry
	t.Cleanup(func() { registry = previous })
	registry = []Type{{Name: "b"}, {Name: "a"}}
	types := All()
	assert.Equal(t, []Type{{Name: "a"}, {Name: "b"}}, types)
	types[0].Name = "changed"
	assert.Equal(t, "b", registry[0].Name, "All returns a copy")
}
