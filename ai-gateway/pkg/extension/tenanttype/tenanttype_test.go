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
	types := All()
	require.Len(t, types, 2)
	assert.Equal(t, []string{"default-instructions", "default-settings"}, []string{types[0].Name, types[1].Name})
	types[0].Example[0] = 'x'
	assert.Equal(t, byte('{'), registry[0].Example[0], "All returns copies")

	previous := registry
	t.Cleanup(func() { registry = previous })
	registry = []Type{{Name: "b"}, {Name: "a"}}
	assert.Equal(t, []Type{{Name: "a"}, {Name: "b"}}, All())
}

func TestShippedTypes(t *testing.T) {
	for _, tc := range []struct {
		tenantType Type
		member     string
		accepted   []string
		refused    []string
	}{
		{
			tenantType: DefaultInstructions(),
			member:     "defaultInstructions",
			accepted:   []string{`{"messages":[{"content":"a"},{"content":"b"}]}`},
			refused: []string{
				`{}`,
				`{"messages":[]}`,
				`{"messages":[{"content":""}]}`,
				`{"messages":[{}]}`,
				`{"messages":[{"content":"a","providerOptions":{}}]}`,
				`{"messages":[{"content":"a","role":"system"}]}`,
				`{"messages":[{"content":"a"}],"providerOptions":{}}`,
				`{"messages":"a"}`,
			},
		},
		{
			tenantType: DefaultSettings(),
			member:     "defaultSettings",
			accepted: []string{
				`{"temperature":0,"topP":1,"topK":40,"maxOutputTokens":1,"stopSequences":[],"seed":-1,"presencePenalty":-0.5,"frequencyPenalty":0.5}`,
				`{"stopSequences":["a","b"]}`,
				`{"maxOutputTokens":1.0,"topK":4e1}`,
			},
			refused: []string{
				`{}`,
				`{"headers":{"x":"y"}}`,
				`{"providerOptions":{"anthropic":{}}}`,
				`{"tools":[]}`,
				`{"toolChoice":{"type":"auto"}}`,
				`{"responseFormat":{"type":"json"}}`,
				`{"reasoning":"high"}`,
				`{"temperature":"0.2"}`,
				`{"maxOutputTokens":0}`,
				`{"topK":1.5}`,
				`{"stopSequences":[1]}`,
			},
		},
	} {
		t.Run(tc.tenantType.Name, func(t *testing.T) {
			assert.Equal(t, tc.member, tc.tenantType.Member())
			assert.False(t, tc.tenantType.EnabledByDefault)
			assert.NotEmpty(t, tc.tenantType.Description)
			require.NotNil(t, tc.tenantType.ConfigSchema.Compiled(), "the schema compiles")
			require.NoError(t, tc.tenantType.ConfigSchema.Validate(tc.tenantType.Example))
			for _, configuration := range tc.accepted {
				assert.NoError(t, tc.tenantType.ConfigSchema.Validate(json.RawMessage(configuration)), configuration)
			}
			for _, configuration := range tc.refused {
				assert.Error(t, tc.tenantType.ConfigSchema.Validate(json.RawMessage(configuration)), configuration)
			}
		})
	}
}

func TestNewType_InvalidSchema(t *testing.T) {
	tenantType := newType("broken", "A broken type.", `{"type":`, json.RawMessage(`{}`))
	assert.Nil(t, tenantType.ConfigSchema.JSON())
	assert.JSONEq(t, `{}`, string(tenantType.Example), "the example stays, so startup refuses the type")
}
