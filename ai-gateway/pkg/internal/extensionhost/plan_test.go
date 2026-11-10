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
