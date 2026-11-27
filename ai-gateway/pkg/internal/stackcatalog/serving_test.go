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

func TestParse_VersionedPublicID(t *testing.T) {
	const id = "claude-sonnet-4@20250514"
	for _, alias := range []bool{false, true} {
		spec := testSpec()
		model := spec.Models["balanced"]
		delete(spec.Models, "balanced")
		if alias {
			model.Aliases = []string{id}
			spec.Models["balanced"] = model
		} else {
			spec.Models[id] = model
		}
		_, err := Parse(spec, testKeys, outbound.CustomEndpoints{})
		require.NoError(t, err)
	}
}

func TestParse_InvalidBlocklist(t *testing.T) {
	const maxBlockedModelIDs = 128
	ids := make([]string, 0, maxBlockedModelIDs)
	for i := range maxBlockedModelIDs {
		ids = append(ids, fmt.Sprintf("model-%d", i))
	}
	ids[0] = strings.Repeat("a", config.MaxPublicIDBytes)
	ids[1] = "claude-sonnet-4@20250514"
	tests := []struct {
		name string
		ids  []string
		want string
	}{
		{name: "at both bounds", ids: ids},
		{name: "too many", ids: append(ids, "one-more"), want: "modelBlocklist allows at most 128 entries"},
		{name: "duplicate", ids: []string{sentinel, sentinel}, want: "modelBlocklist[1] repeats"},
		{name: "empty", ids: []string{""}, want: "modelBlocklist[0]: public ID must be"},
		{name: "blank", ids: []string{" "}, want: "modelBlocklist[0]: public ID must be"},
		{name: "leading whitespace", ids: []string{" " + sentinel}, want: "modelBlocklist[0]: public ID must be"},
		{name: "trailing whitespace", ids: []string{sentinel + " "}, want: "modelBlocklist[0]: public ID must be"},
		{name: "control", ids: []string{sentinel + "\x00"}, want: "modelBlocklist[0]: public ID must be"},
		{name: "invalid UTF-8", ids: []string{sentinel + "\xff"}, want: "modelBlocklist[0]: public ID must be"},
		{name: "leading slash", ids: []string{"/model"}, want: "modelBlocklist[0]: public ID must be"},
		{name: "non ASCII", ids: []string{"modèle"}, want: "modelBlocklist[0]: public ID must be"},
		{name: "too many bytes", ids: []string{ids[0] + "a"}, want: "modelBlocklist[0]: public ID must be"},
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
