package stackcatalog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
)

func testSpec() v1alpha1.Spec {
	description := "General purpose"
	return v1alpha1.Spec{
		Providers: map[string]v1alpha1.Provider{
			"anthropic": {Type: "anthropic"},
			"openai":    {Type: "openai"},
		},
		Models: map[string]v1alpha1.Model{
			"balanced": {
				Name:        "Balanced",
				Description: &description,
				Aliases:     []string{"default"},
				Primary:     v1alpha1.Target{Provider: "anthropic", Model: "claude-sonnet-4-6"},
				Fallback:    []v1alpha1.Target{{Provider: "openai", Model: "gpt-5"}},
			},
		},
	}
}

var testKeys = []string{"anthropic", "openai"}

func TestParse_ConvertsTheSpec(t *testing.T) {
	file, err := Parse(testSpec(), testKeys)
	require.NoError(t, err)
	assert.Equal(t, config.File{
		Providers: map[string]config.Provider{"anthropic": {Type: "anthropic"}, "openai": {Type: "openai"}},
		Models: map[string]config.Model{
			"balanced": {
				Name:        "Balanced",
				Description: "General purpose",
				Aliases:     []string{"default"},
				Primary:     config.Primary{Provider: "anthropic", Model: "claude-sonnet-4-6"},
				Fallback:    []config.Primary{{Provider: "openai", Model: "gpt-5"}},
			},
		},
	}, file)
}

func TestParse_Refuses(t *testing.T) {
	empty, unset := "", false
	withProvider := func(name string, provider v1alpha1.Provider) func(*v1alpha1.Spec) {
		return func(spec *v1alpha1.Spec) { spec.Providers[name] = provider }
	}
	withModel := func(mutate func(*v1alpha1.Model)) func(*v1alpha1.Spec) {
		return func(spec *v1alpha1.Spec) {
			model := spec.Models["balanced"]
			mutate(&model)
			spec.Models["balanced"] = model
		}
	}
	const invalidName = "modelcatalog: provider names must be DNS labels: 1 to 63 lowercase letters, digits or '-', starting and ending with a letter or digit"
	tests := []struct {
		name    string
		mutate  func(*v1alpha1.Spec)
		keys    []string
		wantErr string
	}{
		{name: "uppercase provider name", mutate: withProvider("Anthropic", v1alpha1.Provider{Type: "anthropic"}), wantErr: invalidName},
		{name: "dotted provider name", mutate: withProvider("a.b", v1alpha1.Provider{Type: "anthropic"}), wantErr: invalidName},
		{name: "underscore provider name", mutate: withProvider("a_b", v1alpha1.Provider{Type: "anthropic"}), wantErr: invalidName},
		{name: "leading hyphen", mutate: withProvider("-a", v1alpha1.Provider{Type: "anthropic"}), wantErr: invalidName},
		{name: "provider name too long", mutate: withProvider(strings.Repeat("a", 64), v1alpha1.Provider{Type: "anthropic"}), wantErr: invalidName},
		{name: "compatible type", mutate: withProvider("openai", v1alpha1.Provider{Type: "openai-compatible"}), wantErr: "modelcatalog: providers.openai.type must be anthropic or openai"},
		{name: "unknown type", mutate: withProvider("openai", v1alpha1.Provider{Type: "bedrock"}), wantErr: "modelcatalog: providers.openai.type must be anthropic or openai"},
		{name: "base URL", mutate: withProvider("openai", v1alpha1.Provider{Type: "openai", BaseURL: &empty}), wantErr: "modelcatalog: providers.openai.baseURL is not supported in a model catalog"},
		{name: "provider name field", mutate: withProvider("openai", v1alpha1.Provider{Type: "openai", ProviderName: &empty}), wantErr: "modelcatalog: providers.openai.providerName is not supported in a model catalog"},
		{name: "structured outputs set to false", mutate: withProvider("openai", v1alpha1.Provider{Type: "openai", SupportsStructuredOutputs: &unset}), wantErr: "modelcatalog: providers.openai.supportsStructuredOutputs is not supported in a model catalog"},
		{name: "no providers", mutate: func(spec *v1alpha1.Spec) { spec.Providers = nil }, keys: []string{}, wantErr: "config: at least one provider is required"},
		{name: "no models", mutate: func(spec *v1alpha1.Spec) { spec.Models = nil }, wantErr: "config: at least one model is required"},
		{name: "unknown provider reference", mutate: withModel(func(model *v1alpha1.Model) { model.Primary.Provider = "missing" }), wantErr: `config: models.balanced.primary.provider references unknown provider "missing"`},
		{name: "alias collision", mutate: withModel(func(model *v1alpha1.Model) { model.Aliases = []string{"balanced"} }), wantErr: `config: alias "balanced" collides with canonical model`},
		{name: "missing key", keys: []string{"anthropic"}, wantErr: "modelcatalog: providers.openai requires the secure value secure.openai"},
		{name: "key without provider", keys: []string{"anthropic", "openai", "other"}, wantErr: "modelcatalog: secure.other matches no provider"},
		{name: "key that is not a provider name", keys: []string{"anthropic", "openai", "Not A Name"}, wantErr: "modelcatalog: a secure key that is not a provider name matches no provider"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec := testSpec()
			if tc.mutate != nil {
				tc.mutate(&spec)
			}
			keys := testKeys
			if tc.keys != nil {
				keys = tc.keys
			}
			_, err := Parse(spec, keys)
			assert.EqualError(t, err, tc.wantErr)
		})
	}
}
