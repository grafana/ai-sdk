package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseModelCatalog_ProviderServing(t *testing.T) {
	const providers = "providers:\n  claude: {type: anthropic, apiKeyEnv: EXAMPLE_KEY"
	for _, tc := range []struct{ name, fields, models, want string }{
		{"off", "", "models: {}", "at least one model is required"},
		{"blocklist does not opt in", ", modelBlocklist: [blocked]", "models: {}", "at least one model is required"},
		{"false", ", serveAllModels: false", "models: {}", "at least one model is required"},
		{"on", ", serveAllModels: true, modelBlocklist: [blocked]", "models: {}", ""},
		{"missing", ", serveAllModels: true", "", "models must be a map"},
		{"null map", ", serveAllModels: true", "models: null", "models must be a map"},
		{"wrong boolean", ", serveAllModels: 'true'", "models: {}", "serveAllModels must be a boolean"},
		{"wrong blocklist", ", modelBlocklist: blocked", "models: {}", "modelBlocklist must be a sequence"},
		{"wrong entry", ", modelBlocklist: [123]", "models: {}", "modelBlocklist[0] must be a string"},
		{"duplicate entry", ", serveAllModels: true, modelBlocklist: [blocked, blocked]", "models: {}", "modelBlocklist[1] repeats"},
		{"invalid entry", ", serveAllModels: true, modelBlocklist: [' padded']", "models: {}", "modelBlocklist[0]: public ID must be"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := ParseModelCatalog(providers + tc.fields + "}\n" + tc.models)
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
				assert.NotContains(t, err.Error(), "EXAMPLE_KEY")
				return
			}
			require.NoError(t, err)
			assert.False(t, file.AllowsUnlistedModel("claude", "blocked"))
			assert.True(t, file.AllowsUnlistedModel("claude", "blocked-sequel"))
			assert.True(t, file.AllowsUnlistedModel("claude", "Blocked"))
		})
	}
	_, err := ParseModelCatalog(strings.Replace(providers+", serveAllModels: true}\nmodels: {}", "claude:", "not/a/qualifier:", 1))
	require.ErrorContains(t, err, "serveAllModels requires a valid instance name")
	_, err = ParseModelCatalog("providers:\n  compat: {type: openai-compatible, apiKeyEnv: KEY, baseURL: 'https://example.invalid', serveAllModels: true, providerName: " + strings.Repeat("a", maxStringBytes+1) + "}\nmodels: {}")
	require.ErrorContains(t, err, "providerName is invalid or exceeds the discovery limit")
}

func TestParseModelCatalog_SharedPublicIDs(t *testing.T) {
	const document = `providers:
  team: {type: anthropic, apiKeyEnv: EXAMPLE_KEY, serveAllModels: true, modelBlocklist: [claude-sonnet-4@20250514+preview]}
models:
  sonnet@20250514:
    name: Sonnet
    aliases: [sonnet+preview]
    primary: {provider: team, model: another-backend}
`
	file, err := ParseModelCatalog(document)
	require.NoError(t, err)
	assert.False(t, file.AllowsUnlistedModel("team", "claude-sonnet-4@20250514+preview"))
	assert.True(t, file.AllowsUnlistedModel("team", "claude-sonnet-4@20250514"))
	for _, tc := range []struct{ value, want string }{
		{"sonnet@20250514", "model"},
		{"sonnet+preview", "alias"},
		{"claude-sonnet-4@20250514+preview", "modelBlocklist[0]"},
	} {
		_, err := ParseModelCatalog(strings.Replace(document, tc.value, strings.Repeat("a", MaxPublicIDBytes+1), 1))
		require.ErrorContains(t, err, tc.want)
		require.ErrorContains(t, err, "public ID must be")
	}
}

func TestParseModelCatalog_BlocklistConflicts(t *testing.T) {
	const catalog = `providers:
  claude: {type: anthropic, apiKeyEnv: EXAMPLE_KEY, modelBlocklist: [denied-example]}
  other: {type: anthropic, apiKeyEnv: OTHER_KEY}
models:
  public:
    name: Public
`
	for _, tc := range []struct{ name, route, want string }{
		{"primary", "    primary: {provider: claude, model: denied-example}", "modelBlocklist[0] conflicts with a listed target"},
		{"fallback", "    primary: {provider: other, model: main}\n    fallback: [{provider: claude, model: denied-example}]", "modelBlocklist[0] conflicts with a listed target"},
		{"server primary", "    primary: {provider: claude, model: main, serverFallbacks: {models: [denied-example]}}", "primary.serverFallbacks.models[0] conflicts"},
		{"server fallback", "    primary: {provider: other, model: main}\n    fallback: [{provider: claude, model: secondary, serverFallbacks: {models: [denied-example]}}]", "fallback[0].serverFallbacks.models[0] conflicts"},
		{"vendor default", "    primary: {provider: claude, model: main, serverFallbacks: {allowDefault: true}}", "serverFallbacks.allowDefault is incompatible"},
		{"other instance", "    primary: {provider: other, model: denied-example, serverFallbacks: {allowDefault: true}}", ""},
		{"case and prefix", "    primary: {provider: claude, model: denied-example-next, serverFallbacks: {models: [Denied-example]}}", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModelCatalog(catalog + tc.route)
			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.want)
			assert.NotContains(t, err.Error(), "denied-example")
		})
	}
}
