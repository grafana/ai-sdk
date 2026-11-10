package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseExtensions(t *testing.T) {
	enabled, disabled := true, false
	for _, tc := range []struct {
		name     string
		document string
		want     map[string]ExtensionEntry
	}{
		{name: "blank", document: " \n"},
		{name: "entries", document: "quota:\n  config:\n    limit: 100\n    tiers: [a, b]\n  apiKeyEnv: GF_PLUGIN_QUOTA_KEY\naudit:\n  enabled: false\nplain: {}\n", want: map[string]ExtensionEntry{
			"quota": {Config: json.RawMessage(`{"limit":100,"tiers":["a","b"]}`), SecretEnv: map[string]string{"apiKey": "GF_PLUGIN_QUOTA_KEY"}},
			"audit": {Enabled: &disabled},
			"plain": {},
		}},
		{name: "explicit enable", document: "quota: {enabled: true, config: {}}", want: map[string]ExtensionEntry{
			"quota": {Enabled: &enabled, Config: json.RawMessage(`{}`)},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseExtensions(tc.document)
			require.NoError(t, err)
			require.Len(t, got, len(tc.want))
			for name, want := range tc.want {
				entry := got[name]
				assert.Equal(t, want.Enabled, entry.Enabled, name)
				assert.Equal(t, want.SecretEnv, entry.SecretEnv, name)
				if want.Config == nil {
					assert.Nil(t, entry.Config, name)
				} else {
					assert.JSONEq(t, string(want.Config), string(entry.Config), name)
				}
			}
		})
	}
}

func TestParseExtensions_Refusals(t *testing.T) {
	for _, tc := range []struct {
		name     string
		document string
		message  string
	}{
		{name: "oversized", document: "quota: {config: {v: " + strings.Repeat("x", MaxExtensionsBytes) + "}}", message: "GF_PLUGIN_EXTENSIONS exceeds"},
		{name: "not YAML", document: "quota: [", message: "GF_PLUGIN_EXTENSIONS is not valid YAML"},
		{name: "sequence root", document: "- quota", message: "must be a mapping of extension names"},
		{name: "trailing document", document: "quota: {}\n---\naudit: {}\n", message: "must be one YAML document"},
		{name: "alias", document: "base: &b {config: {secret-value: 1}}\nquota: *b\n", message: "aliases are not supported"},
		{name: "merge key", document: "quota:\n  <<: {enabled: true}\n", message: "aliases are not supported"},
		{name: "entry not a mapping", document: "quota: secret-value", message: "GF_PLUGIN_EXTENSIONS.quota must be a mapping"},
		{name: "unknown key", document: "quota: {limit: secret-value}", message: "GF_PLUGIN_EXTENSIONS.quota.limit is not a supported field"},
		{name: "bare Env key", document: "quota: {Env: secret-value}", message: "GF_PLUGIN_EXTENSIONS.quota.Env is not a supported field"},
		{name: "enabled not a boolean", document: "quota: {enabled: secret-value}", message: "GF_PLUGIN_EXTENSIONS.quota.enabled must be a boolean"},
		{name: "config not a mapping", document: "quota: {config: [secret-value]}", message: "GF_PLUGIN_EXTENSIONS.quota.config must be a mapping"},
		{name: "config with a non-string key", document: "quota: {config: {1: secret-value}}", message: "GF_PLUGIN_EXTENSIONS.quota.config must be a mapping"},
		{name: "empty secret variable", document: "quota: {apiKeyEnv: ' '}", message: "GF_PLUGIN_EXTENSIONS.quota.apiKeyEnv must name an environment variable"},
		{name: "repeated entry", document: "quota: {}\nquota: {}\n", message: "GF_PLUGIN_EXTENSIONS.quota is repeated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseExtensions(tc.document)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.message)
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}

func TestParseSettings_Extensions(t *testing.T) {
	env := map[string]string{"GF_PLUGIN_MODEL_CATALOG": "models: []", "GF_PLUGIN_ACCESS_MODE": "self-hosted", "GF_PLUGIN_EXTENSIONS": "quota: {}"}
	settings, err := ParseSettings(func(name string) (string, bool) { value, ok := env[name]; return value, ok })
	require.NoError(t, err)
	assert.Equal(t, "quota: {}", settings.Extensions)
}
