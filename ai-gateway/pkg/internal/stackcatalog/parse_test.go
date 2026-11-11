package stackcatalog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
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

func pointer[T any](value T) *T { return &value }

func testPolicy(t *testing.T) outbound.EgressPolicy {
	t.Helper()
	allow, err := outbound.ParseEgressList("10.20.0.0/16")
	require.NoError(t, err)
	deny, err := outbound.ParseEgressList("denied.example,*.blocked.example,8.8.8.8")
	require.NoError(t, err)
	return outbound.EgressPolicy{Allow: allow, Deny: deny}
}

func TestParse_ConvertsTheSpec(t *testing.T) {
	file, err := Parse(testSpec(), testKeys, outbound.EgressPolicy{})
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

func TestParse_CustomEndpoints(t *testing.T) {
	spec := testSpec()
	spec.Providers["anthropic"] = v1alpha1.Provider{Type: "anthropic", BaseURL: pointer("HTTPS://Anthropic-Proxy.example.com/v1")}
	spec.Providers["local"] = v1alpha1.Provider{Type: "openai-compatible", BaseURL: pointer("https://10.20.0.5:8443/v1"), ProviderName: pointer("acme"), SupportsStructuredOutputs: pointer(true)}
	spec.Providers["openai"] = v1alpha1.Provider{Type: "openai", BaseURL: pointer("https://[2606:4700::1111]/v1")}
	model := spec.Models["balanced"]
	model.Fallback = append(model.Fallback, v1alpha1.Target{Provider: "local", Model: "llama"})
	spec.Models["balanced"] = model
	file, err := Parse(spec, []string{"anthropic", "local", "openai"}, testPolicy(t))
	require.NoError(t, err)
	assert.Equal(t, map[string]config.Provider{
		"anthropic": {Type: "anthropic", BaseURL: "https://Anthropic-Proxy.example.com/v1"},
		"local":     {Type: "openai-compatible", BaseURL: "https://10.20.0.5:8443/v1", ProviderName: "acme", SupportsStructuredOutputs: true},
		"openai":    {Type: "openai", BaseURL: "https://[2606:4700::1111]/v1"},
	}, file.Providers)
}

func TestParse_RefusesEndpoints(t *testing.T) {
	const (
		form     = "modelcatalog: providers.openai.baseURL must be an absolute URL without userinfo, query or fragment"
		https    = "modelcatalog: providers.openai.baseURL must use https"
		refused  = "modelcatalog: providers.openai.baseURL is refused by the egress policy"
		hostForm = "modelcatalog: providers.openai.baseURL host must be a DNS name or an IP address"
	)
	for raw, want := range map[string]string{
		"http://llm.example.com":                    https,
		"ftp://llm.example.com":                     form,
		"https://user:private-secret@llm.example":   form,
		"https://llm.example.com/v1?key=private":    form,
		"https://llm.example.com/v1#private":        form,
		"/v1":                                       form,
		"https:///v1":                               form,
		"  ":                                        form,
		"https://127.0.0.1":                         refused,
		"https://169.254.169.254/v1":                refused,
		"https://[::ffff:10.0.0.1]":                 refused,
		"https://[::1]:8443":                        refused,
		"https://[fe80::1%25eth0]":                  refused,
		"https://[64:ff9b::a00:1]":                  refused,
		"https://10.21.0.1":                         refused,
		"https://8.8.8.8":                           refused,
		"https://denied.example/v1":                 refused,
		"https://DENIED.example./v1":                refused,
		"https://api.blocked.example":               refused,
		"https://127.1":                             hostForm,
		"https://2130706433":                        hostForm,
		"https://0x7f000001":                        hostForm,
		"https://017700000001":                      hostForm,
		"https://llm_corp.example":                  hostForm,
		"https://xn--nxasmq6b.example":              "",
		"https://llm.example.com:8443/v1/":          "",
		"https://10.20.0.1/v1":                      "",
		"https://2606:4700::1111":                   form,
		"https://%31%32%37.0.0.1":                   form,
		"https://llm.example.com\\@169.254.169.254": form,
	} {
		t.Run(raw, func(t *testing.T) {
			spec := testSpec()
			spec.Providers["openai"] = v1alpha1.Provider{Type: "openai", BaseURL: pointer(raw)}
			_, err := Parse(spec, testKeys, testPolicy(t))
			if want == "" {
				require.NoError(t, err)
				return
			}
			assert.EqualError(t, err, want)
			assert.NotContains(t, err.Error(), "private")
		})
	}
}

func TestParse_Refuses(t *testing.T) {
	empty := ""
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
		{name: "compatible type without an endpoint", mutate: withProvider("openai", v1alpha1.Provider{Type: "openai-compatible"}), wantErr: "config: providers.openai.baseURL is required for openai-compatible providers"},
		{name: "compatible type with an empty endpoint", mutate: withProvider("openai", v1alpha1.Provider{Type: "openai-compatible", BaseURL: &empty}), wantErr: "config: providers.openai.baseURL is required for openai-compatible providers"},
		{name: "unknown type", mutate: withProvider("openai", v1alpha1.Provider{Type: "bedrock"}), wantErr: "modelcatalog: providers.openai.type must be anthropic, openai or openai-compatible"},
		{name: "provider name on a native type", mutate: withProvider("openai", v1alpha1.Provider{Type: "openai", ProviderName: pointer("acme")}), wantErr: "config: providers.openai.providerName is only supported for openai-compatible providers"},
		{name: "structured outputs on a native type", mutate: withProvider("openai", v1alpha1.Provider{Type: "openai", SupportsStructuredOutputs: pointer(true)}), wantErr: "config: providers.openai.supportsStructuredOutputs is only supported for openai-compatible providers"},
		{name: "reserved provider name", mutate: withProvider("openai", v1alpha1.Provider{Type: "openai-compatible", BaseURL: pointer("https://llm.example.com"), ProviderName: pointer("gateway")}), wantErr: `config: providers.openai.providerName "gateway" is reserved by the runtime as the host provider-option namespace; rename the provider`},
		{name: "blank provider name", mutate: withProvider("openai", v1alpha1.Provider{Type: "openai-compatible", BaseURL: pointer("https://llm.example.com"), ProviderName: pointer(" ")}), wantErr: "config: providers.openai.providerName must not be blank"},
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
			_, err := Parse(spec, keys, outbound.EgressPolicy{})
			assert.EqualError(t, err, tc.wantErr)
		})
	}
}
