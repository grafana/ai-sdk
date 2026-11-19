package stackcatalog

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
)

func TestParse_ProviderServingPolicy(t *testing.T) {
	spec := testSpec()
	spec.Providers["openai"] = v1alpha1.Provider{Type: "openai", ServeAllModels: new(true), ModelBlocklist: []string{"org/blocked"}}
	spec.Providers["second"] = v1alpha1.Provider{Type: "openai", ServeAllModels: new(true), ModelBlocklist: []string{"gpt-5"}}
	spec.Providers["off"] = v1alpha1.Provider{Type: "openai", ServeAllModels: new(false), ModelBlocklist: []string{"org/blocked"}}
	parsed, err := Parse(spec, []string{"anthropic", "openai", "second", "off"}, outbound.CustomEndpoints{})
	require.NoError(t, err)
	assert.False(t, parsed.AllowsUnlistedModel("openai", "org/blocked"))
	assert.True(t, parsed.AllowsUnlistedModel("openai", "org/blocked-next"))
	assert.True(t, parsed.AllowsUnlistedModel("openai", "org/Blocked"))
	assert.True(t, parsed.AllowsUnlistedModel("second", "org/blocked"))
	assert.False(t, parsed.AllowsUnlistedModel("second", "gpt-5"))
	assert.False(t, parsed.AllowsUnlistedModel("off", "unlisted"))
	assert.False(t, parsed.AllowsUnlistedModel("anthropic", "unlisted"))
	assert.False(t, parsed.AllowsUnlistedModel("unknown", "unlisted"))
}

func TestParse_InvalidBlocklist(t *testing.T) {
	ids := make([]string, 0, maxBlockedModelIDs)
	for i := range maxBlockedModelIDs {
		ids = append(ids, fmt.Sprintf("model-%d", i))
	}
	ids[0] = strings.Repeat("é", maxBackendIDBytes/2)
	tests := []struct {
		name string
		ids  []string
		want string
	}{
		{name: "at both bounds", ids: ids},
		{name: "too many", ids: append(ids, "one-more"), want: "modelBlocklist allows at most 128 entries"},
		{name: "duplicate", ids: []string{sentinel, sentinel}, want: "modelBlocklist[1] repeats"},
		{name: "empty", ids: []string{""}, want: "modelBlocklist[0] must be"},
		{name: "blank", ids: []string{" "}, want: "modelBlocklist[0] must be"},
		{name: "leading whitespace", ids: []string{" " + sentinel}, want: "modelBlocklist[0] must be"},
		{name: "trailing whitespace", ids: []string{sentinel + " "}, want: "modelBlocklist[0] must be"},
		{name: "control", ids: []string{sentinel + "\x00"}, want: "modelBlocklist[0] must be"},
		{name: "invalid UTF-8", ids: []string{sentinel + "\xff"}, want: "modelBlocklist[0] must be"},
		{name: "too many bytes", ids: []string{ids[0] + "a"}, want: "modelBlocklist[0] must be"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := testSpec()
			spec.Providers["openai"] = v1alpha1.Provider{Type: "openai", ModelBlocklist: tc.ids}
			_, err := Parse(spec, testKeys, outbound.CustomEndpoints{})
			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, "providers.openai."+tc.want)
			assert.NotContains(t, err.Error(), sentinel)
		})
	}
}

func TestParse_OperatorCatalogDoesNotAcceptServingFields(t *testing.T) {
	for _, field := range []string{"serveAllModels: true", "modelBlocklist: [blocked]"} {
		t.Run(field, func(t *testing.T) {
			_, err := config.ParseModelCatalog(`providers:
  openai:
    type: openai
    apiKeyEnv: OPENAI_API_KEY
    ` + field + `
models:
  model:
    name: Model
    primary: {provider: openai, model: gpt-5}
`)
			assert.ErrorContains(t, err, "invalid YAML fields or values")
		})
	}
}
