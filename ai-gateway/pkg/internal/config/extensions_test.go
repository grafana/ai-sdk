package config

import (
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
		{name: "entries", document: "quota:\n  config:\n    limit: 100\n    tiers: [a, b]\n  secrets:\n    apiKey: GF_PLUGIN_QUOTA_KEY\naudit:\n  enabled: false\nplain: {}\n", want: map[string]ExtensionEntry{
			"quota": {Config: map[string]any{"limit": 100, "tiers": []any{"a", "b"}}, Secrets: map[string]string{"apiKey": "GF_PLUGIN_QUOTA_KEY"}},
			"audit": {Enabled: &disabled},
			"plain": {},
		}},
		{name: "explicit enable", document: "quota: {enabled: true, config: {}}", want: map[string]ExtensionEntry{
			"quota": {Enabled: &enabled, Config: map[string]any{}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseExtensions(tc.document)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
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
		{name: "sequence root", document: "- quota", message: "YAML root must be a mapping in GF_PLUGIN_EXTENSIONS"},
		{name: "non-string name", document: "1: {}", message: "YAML root mapping keys must be strings in GF_PLUGIN_EXTENSIONS"},
		{name: "trailing document", document: "quota: {}\n---\naudit: {}\n", message: "GF_PLUGIN_EXTENSIONS must be one YAML document"},
		{name: "alias", document: "base: &b {config: {secret-value: 1}}\nquota: *b\n", message: "aliases are not supported in GF_PLUGIN_EXTENSIONS"},
		{name: "merge key", document: "quota:\n  <<: {enabled: true}\n", message: "aliases are not supported in GF_PLUGIN_EXTENSIONS"},
		{name: "entry not a mapping", document: "quota: secret-value", message: "GF_PLUGIN_EXTENSIONS has invalid fields or values"},
		{name: "unknown key", document: "quota: {limit: secret-value}", message: "GF_PLUGIN_EXTENSIONS has invalid fields or values"},
		{name: "enabled not a boolean", document: "quota: {enabled: secret-value}", message: "GF_PLUGIN_EXTENSIONS has invalid fields or values"},
		{name: "config not a mapping", document: "quota: {config: [secret-value]}", message: "GF_PLUGIN_EXTENSIONS has invalid fields or values"},
		{name: "secrets not a mapping", document: "quota: {secrets: [secret-value]}", message: "GF_PLUGIN_EXTENSIONS has invalid fields or values"},
		{name: "repeated entry", document: "quota: {}\nquota: {}\n", message: "GF_PLUGIN_EXTENSIONS has invalid fields or values"},
		{name: "repeated config key", document: "quota: {config: {limit: 1, limit: secret-value}}", message: "GF_PLUGIN_EXTENSIONS has invalid fields or values"},
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
