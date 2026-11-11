package extensionhost

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/schema"
)

type quotaConfig struct {
	Limit int `json:"limit" jsonschema:"minimum=1"`
}

func quotaDefinition(t *testing.T) extension.Definition {
	t.Helper()
	definition := validDefinition("quota")
	configSchema, err := schema.SchemaFor[quotaConfig]()
	require.NoError(t, err)
	definition.ConfigSchema = configSchema
	definition.Secrets = []string{"apiKey"}
	return definition
}

func parseEntries(t *testing.T, document string) map[string]config.ExtensionEntry {
	t.Helper()
	entries, err := config.ParseExtensions(document)
	require.NoError(t, err)
	return entries
}

func TestNewPlan(t *testing.T) {
	tenant := validDefinition("instructions")
	tenant.Scope, tenant.EnabledByDefault = extension.ScopeTenant, true
	defaultOn := validDefinition("audit")
	defaultOn.EnabledByDefault = true
	off := validDefinition("off")

	plan, err := NewPlan([]extension.Definition{quotaDefinition(t), tenant, defaultOn, off}, []string{"quota"}, parseEntries(t, "quota: {config: {limit: 3}, apiKeyEnv: QUOTA_KEY}\n"))
	require.NoError(t, err)
	enabled := map[string]json.RawMessage{}
	for _, planned := range plan.enabled {
		enabled[planned.definition.Name] = planned.config
	}
	assert.Equal(t, map[string]json.RawMessage{"quota": json.RawMessage(`{"limit":3}`), "instructions": emptyConfig, "audit": emptyConfig}, enabled)

	disabled, err := NewPlan([]extension.Definition{defaultOn}, nil, parseEntries(t, "audit: {enabled: false}"))
	require.NoError(t, err)
	assert.Empty(t, disabled.enabled)
}

func TestNewPlan_Refusals(t *testing.T) {
	tenant := validDefinition("instructions")
	tenant.Scope = extension.ScopeTenant
	withSecretDefault := validDefinition("quota")
	withSecretDefault.EnabledByDefault, withSecretDefault.Secrets = true, []string{"apiKey"}
	defaultOnMandatory := validDefinition("audit")
	defaultOnMandatory.EnabledByDefault = true
	withRequiredConfig := quotaDefinition(t)
	withRequiredConfig.EnabledByDefault, withRequiredConfig.Secrets = true, nil
	for _, tc := range []struct {
		name        string
		definitions []extension.Definition
		mandatory   []string
		document    string
		message     string
	}{
		{name: "unknown extension", definitions: []extension.Definition{validDefinition("audit")}, document: "quota: {}", message: "GF_PLUGIN_EXTENSIONS.quota is not a registered extension"},
		{name: "tenant extension", definitions: []extension.Definition{tenant}, document: "instructions: {}", message: "GF_PLUGIN_EXTENSIONS.instructions is a tenant extension"},
		{name: "undeclared secret", definitions: []extension.Definition{quotaDefinition(t)}, document: "quota: {tokenEnv: SECRET_VALUE}", message: "GF_PLUGIN_EXTENSIONS.quota.tokenEnv is not a declared secret"},
		{name: "mandatory unconfigured", definitions: []extension.Definition{validDefinition("audit")}, mandatory: []string{"audit"}, message: "GF_PLUGIN_EXTENSIONS.audit is mandatory"},
		{name: "mandatory enabled by default without an entry", definitions: []extension.Definition{defaultOnMandatory}, mandatory: []string{"audit"}, message: "GF_PLUGIN_EXTENSIONS.audit is mandatory"},
		{name: "mandatory disabled", definitions: []extension.Definition{validDefinition("audit")}, mandatory: []string{"audit"}, document: "audit: {enabled: false}", message: "GF_PLUGIN_EXTENSIONS.audit is mandatory"},
		{name: "mandatory invalid", definitions: []extension.Definition{quotaDefinition(t)}, mandatory: []string{"quota"}, document: "quota: {config: {limit: 0}, apiKeyEnv: QUOTA_KEY}", message: "GF_PLUGIN_EXTENSIONS.quota.config does not match the extension's schema at /limit"},
		{name: "configuration of the wrong type", definitions: []extension.Definition{quotaDefinition(t)}, document: "quota: {config: {limit: secret-value}, apiKeyEnv: QUOTA_KEY}", message: "does not match the extension's schema at /limit"},
		{name: "configuration without a schema", definitions: []extension.Definition{validDefinition("audit")}, document: "audit: {config: {level: secret-value}}", message: "GF_PLUGIN_EXTENSIONS.audit.config is not accepted"},
		{name: "missing secret", definitions: []extension.Definition{quotaDefinition(t)}, document: "quota: {config: {limit: 1}}", message: "GF_PLUGIN_EXTENSIONS.quota.apiKeyEnv is required"},
		{name: "default-enabled without its secret", definitions: []extension.Definition{withSecretDefault}, message: "GF_PLUGIN_EXTENSIONS.quota.apiKeyEnv is required"},
		{name: "default-enabled with required configuration", definitions: []extension.Definition{withRequiredConfig}, message: "GF_PLUGIN_EXTENSIONS.quota.config does not match the extension's schema at /"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewPlan(tc.definitions, tc.mandatory, parseEntries(t, tc.document))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
			assert.NotContains(t, err.Error(), "secret-value")
			assert.NotContains(t, err.Error(), "SECRET_VALUE")
		})
	}
}

func TestPlan_ResolveSecrets(t *testing.T) {
	definitions := []extension.Definition{quotaDefinition(t)}
	lookup := func(values map[string]string) config.LookupEnv {
		return func(name string) (string, bool) {
			value, ok := values[name]
			return value, ok
		}
	}

	plan, err := NewPlan(definitions, nil, parseEntries(t, "quota: {config: {limit: 1}, apiKeyEnv: QUOTA_KEY}"))
	require.NoError(t, err)
	require.NoError(t, plan.ResolveSecrets(lookup(map[string]string{"QUOTA_KEY": "resolved-secret"})))
	assert.Equal(t, []string{"resolved-secret"}, plan.Secrets())
	assert.Equal(t, map[string]string{"apiKey": "resolved-secret"}, plan.enabled[0].secrets)

	for _, value := range map[string]map[string]string{"unset": {}, "blank": {"QUOTA_KEY": " "}} {
		err := plan.ResolveSecrets(lookup(value))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `GF_PLUGIN_EXTENSIONS.quota.apiKeyEnv "QUOTA_KEY" is unset or empty`)
	}

	disabled, err := NewPlan(definitions, nil, parseEntries(t, "quota: {enabled: false, apiKeyEnv: UNSET_KEY}"))
	require.NoError(t, err)
	assert.NoError(t, disabled.ResolveSecrets(lookup(map[string]string{})))
	assert.Empty(t, disabled.Secrets())
}

func TestCheckConfig(t *testing.T) {
	const sentinel = "sentinel-config-value"
	configSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"limit":{"type":"integer","minimum":1},"items":{"type":"array","items":{"type":"string","maxLength":3}},"perModel":{"type":"object","additionalProperties":{"type":"object","properties":{"max":{"type":"integer","minimum":1}},"additionalProperties":false}},"rules":{"type":"array","items":{"$ref":"#/$defs/rule"}},"choice":{"oneOf":[{"type":"object","additionalProperties":{"type":"integer"}},{"type":"string"}]}},"additionalProperties":false,"$defs":{"rule":{"type":"object","properties":{"tags":{"type":"object","additionalProperties":{"type":"string","maxLength":2}}},"additionalProperties":false}}}`))
	require.NoError(t, err)
	withSchema := validDefinition("quota")
	withSchema.ConfigSchema = configSchema
	withoutSchema := validDefinition("audit")
	untypedSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"limits":{"items":{"type":"integer"},"additionalProperties":{"type":"integer","minimum":1}}}}`))
	require.NoError(t, err)
	untyped := validDefinition("untyped")
	untyped.ConfigSchema = untypedSchema
	exactSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer","maximum":9007199254740992},"m":{"type":"integer","multipleOf":2},"big":{"type":"integer","maximum":9223372036854775807},"set":{"type":"array","uniqueItems":true,"items":{"type":"integer"}}},"additionalProperties":false}`))
	require.NoError(t, err)
	exact := validDefinition("exact")
	exact.ConfigSchema = exactSchema
	for _, tc := range []struct {
		name          string
		definition    extension.Definition
		configuration string
		message       string
	}{
		{name: "valid", definition: withSchema, configuration: `{"limit":2,"items":["a"]}`},
		{name: "empty without schema", definition: withoutSchema, configuration: `{}`},
		{name: "configuration without schema", definition: withoutSchema, configuration: `{"limit":"` + sentinel + `"}`, message: "is not accepted: the extension takes no configuration"},
		{name: "wrong value type", definition: withSchema, configuration: `{"limit":"` + sentinel + `"}`, message: "does not match the extension's schema at /limit"},
		{name: "array item", definition: withSchema, configuration: `{"items":["a","` + sentinel + `"]}`, message: "does not match the extension's schema at /items/1"},
		{name: "undeclared key", definition: withSchema, configuration: `{"` + sentinel + `":1}`, message: "does not match the extension's schema at /"},
		{name: "typed map key", definition: withSchema, configuration: `{"perModel":{"` + sentinel + `":{"max":0}}}`, message: "does not match the extension's schema at /perModel/*/max"},
		{name: "typed map key behind a reference", definition: withSchema, configuration: `{"rules":[{"tags":{"` + sentinel + `":"long"}}]}`, message: "does not match the extension's schema at /rules/0/tags/*"},
		{name: "key under a combinator", definition: withSchema, configuration: `{"choice":{"` + sentinel + `":"x"}}`, message: "does not match the extension's schema at /choice/*"},
		{name: "digit key where the schema also allows an array", definition: untyped, configuration: `{"limits":{"4111111111111111":0}}`, message: "does not match the extension's schema at /limits/*"},
		{name: "integer above 2^53 over a bound", definition: exact, configuration: `{"n":9007199254740993}`, message: "does not match the extension's schema at /n"},
		{name: "odd integer above 2^53", definition: exact, configuration: `{"m":9007199254740993}`, message: "does not match the extension's schema at /m"},
		{name: "integer at the int64 bound", definition: exact, configuration: `{"big":9223372036854775807}`},
		{name: "distinct integers above 2^53", definition: exact, configuration: `{"set":[9007199254740992,9007199254740993]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckConfig(tc.definition, json.RawMessage(tc.configuration))
			if tc.message == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tc.message, err.Error())
			assert.NotContains(t, err.Error(), sentinel)
		})
	}
}
