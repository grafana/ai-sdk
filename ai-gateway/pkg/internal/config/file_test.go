package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimalConfigYAML = `providers:
  anthropic-primary:
    type: anthropic
    apiKeyEnv: ANTHROPIC_API_KEY
models:
  grafana/assistant:
    name: Grafana Assistant
    description: General-purpose assistant
    primary:
      provider: anthropic-primary
      model: claude-test
    aliases:
      - assistant
`

func TestParseModelCatalog_StrictBoundedDocument(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		file, err := ParseModelCatalog(minimalConfigYAML)
		require.NoError(t, err)
		require.Len(t, file.Models, 1)
		assert.Equal(t, []string{"assistant"}, file.Models["grafana/assistant"].Aliases)
	})

	t.Run("single-line JSON", func(t *testing.T) {
		file, err := ParseModelCatalog(`{"providers":{"anthropic-primary":{"type":"anthropic","apiKeyEnv":"ANTHROPIC_API_KEY"}},"models":{"grafana/assistant":{"name":"Grafana Assistant","primary":{"provider":"anthropic-primary","model":"claude-test"},"aliases":["assistant"]}}}`)
		require.NoError(t, err)
		assert.Equal(t, []string{"assistant"}, file.Models["grafana/assistant"].Aliases)
	})

	t.Run("exact limit", func(t *testing.T) {
		require.Equal(t, 64<<10, MaxModelCatalogBytes)
		padded := minimalConfigYAML + "#" + strings.Repeat("x", MaxModelCatalogBytes-len(minimalConfigYAML)-1)
		require.Len(t, padded, MaxModelCatalogBytes)
		_, err := ParseModelCatalog(padded)
		require.NoError(t, err)
	})

	t.Run("over limit", func(t *testing.T) {
		oversized := minimalConfigYAML + "#" + strings.Repeat("x", MaxModelCatalogBytes-len(minimalConfigYAML))
		_, err := ParseModelCatalog(oversized)
		require.EqualError(t, err, "config: GF_PLUGIN_MODEL_CATALOG exceeds 65536 bytes")
	})

	tests := []struct {
		name string
		yaml string
	}{
		{name: "unknown root field", yaml: minimalConfigYAML + "unknown: true\n"},
		{name: "unknown provider field", yaml: strings.Replace(minimalConfigYAML, "    apiKeyEnv:", "    apiKey: literal\n    apiKeyEnv:", 1)},
		{name: "duplicate root key", yaml: minimalConfigYAML + "models: {}\n"},
		{name: "duplicate nested key", yaml: strings.Replace(minimalConfigYAML, "    type: anthropic", "    type: anthropic\n    type: anthropic", 1)},
		{name: "trailing document", yaml: minimalConfigYAML + "---\n{}\n"},
		{name: "empty trailing document", yaml: minimalConfigYAML + "---\n"},
		{name: "empty providers", yaml: "providers: {}\nmodels: {}\n"},
		{name: "empty models", yaml: "providers:\n  p:\n    type: anthropic\n    apiKeyEnv: KEY\nmodels: {}\n"},
		{name: "unknown provider type", yaml: strings.Replace(minimalConfigYAML, "type: anthropic", "type: unsupported", 1)},
		{name: "openai-compatible without base URL", yaml: strings.Replace(minimalConfigYAML, "type: anthropic", "type: openai-compatible", 1)},
		{name: "provider name on anthropic", yaml: strings.Replace(minimalConfigYAML, "    apiKeyEnv: ANTHROPIC_API_KEY\n", "    apiKeyEnv: ANTHROPIC_API_KEY\n    providerName: other\n", 1)},
		{name: "provider name on openai", yaml: strings.Replace(strings.Replace(minimalConfigYAML, "type: anthropic", "type: openai", 1), "    apiKeyEnv: ANTHROPIC_API_KEY\n", "    apiKeyEnv: ANTHROPIC_API_KEY\n    providerName: other\n", 1)},
		{name: "blank compatible provider name", yaml: strings.Replace(minimalConfigYAML, "    type: anthropic", "    type: openai-compatible\n    baseURL: https://backend.invalid\n    providerName: ' '", 1)},
		{name: "structured outputs on anthropic", yaml: strings.Replace(minimalConfigYAML, "    apiKeyEnv: ANTHROPIC_API_KEY\n", "    apiKeyEnv: ANTHROPIC_API_KEY\n    supportsStructuredOutputs: true\n", 1)},
		{name: "structured outputs on openai", yaml: strings.Replace(strings.Replace(minimalConfigYAML, "type: anthropic", "type: openai", 1), "    apiKeyEnv: ANTHROPIC_API_KEY\n", "    apiKeyEnv: ANTHROPIC_API_KEY\n    supportsStructuredOutputs: true\n", 1)},
		{name: "missing api key reference", yaml: strings.Replace(minimalConfigYAML, "    apiKeyEnv: ANTHROPIC_API_KEY\n", "", 1)},
		{name: "missing model name", yaml: strings.Replace(minimalConfigYAML, "    name: Grafana Assistant\n", "", 1)},
		{name: "unknown provider reference", yaml: strings.Replace(minimalConfigYAML, "provider: anthropic-primary", "provider: missing", 1)},
		{name: "missing backend model", yaml: strings.Replace(minimalConfigYAML, "      model: claude-test\n", "", 1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModelCatalog(tc.yaml)
			require.Error(t, err)
		})
	}
}

func TestParseModelCatalog_RejectsLegacyAuthenticationBlocks(t *testing.T) {
	for name, block := range map[string]string{
		"jwt auth":        "auth:\n  type: jwt\n  jwt:\n    jwksURL: https://private-jwks.example/keys\n",
		"static-key auth": "auth:\n  type: static-key\n  staticKey:\n    identities:\n      private-identity:\n        keyEnv: PRIVATE_KEY_ENV\n",
		"cloud server":    "server:\n  cloud:\n    enabled: false\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseModelCatalog(minimalConfigYAML + block)
			require.EqualError(t, err, "config: invalid YAML fields or values")
		})
	}
}

func TestValidatePublicID(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "simple", value: "assistant", valid: true},
		{name: "planned", value: "grafana/assistant:model-v1.2", valid: true},
		{name: "maximum", value: "a" + strings.Repeat("-", 127), valid: true},
		{name: "empty", value: ""},
		{name: "too long", value: "a" + strings.Repeat("-", 128)},
		{name: "leading punctuation", value: "/assistant"},
		{name: "space", value: "grafana assistant"},
		{name: "control", value: "grafana\nassistant"},
		{name: "comma", value: "grafana,assistant"},
		{name: "non ascii", value: "grafaná"},
		{name: "invalid punctuation", value: "grafana@assistant"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePublicID(tc.value)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestFileValidate_PublicIDCollisions(t *testing.T) {
	base := testModelConfig()
	tests := []struct {
		name   string
		mutate func(*File)
	}{
		{name: "unsafe canonical", mutate: func(file *File) { file.Models["bad id"] = file.Models["canonical"]; delete(file.Models, "canonical") }},
		{name: "unsafe alias", mutate: func(file *File) {
			model := file.Models["canonical"]
			model.Aliases = []string{"bad,id"}
			file.Models["canonical"] = model
		}},
		{name: "duplicate alias", mutate: func(file *File) {
			model := file.Models["canonical"]
			model.Aliases = []string{"alias", "alias"}
			file.Models["canonical"] = model
		}},
		{name: "alias canonical collision", mutate: func(file *File) {
			model := file.Models["canonical"]
			model.Aliases = []string{"other"}
			file.Models["canonical"] = model
			file.Models["other"] = Model{Name: "Other", Primary: Primary{Provider: "provider", Model: "other"}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file := cloneFile(base)
			tc.mutate(&file)
			require.Error(t, file.Validate())
		})
	}
}

func TestResolveProviderSecrets(t *testing.T) {
	file := File{Providers: map[string]Provider{
		"one": {Type: "anthropic", APIKeyEnv: "SHARED_KEY", BaseURL: "https://one.example"},
		"two": {Type: "anthropic", APIKeyEnv: "SHARED_KEY"},
	}}

	t.Run("resolves each environment reference once", func(t *testing.T) {
		calls := 0
		resolved, err := file.ResolveProviderSecrets(func(name string) (string, bool) {
			calls++
			assert.Equal(t, "SHARED_KEY", name)
			return "secret-value", true
		})
		require.NoError(t, err)
		assert.Equal(t, 1, calls)
		assert.Equal(t, "secret-value", resolved["one"].APIKey)
		assert.Equal(t, "https://one.example", resolved["one"].BaseURL)
		assert.Equal(t, "secret-value", resolved["two"].APIKey)
	})

	for _, tc := range []struct {
		name   string
		lookup LookupEnv
	}{
		{name: "unset", lookup: func(string) (string, bool) { return "", false }},
		{name: "empty", lookup: func(string) (string, bool) { return "", true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := file.ResolveProviderSecrets(tc.lookup)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "SHARED_KEY")
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}

func TestParseModelCatalog_OpenAIProviderBaseURLOptional(t *testing.T) {
	_, err := ParseModelCatalog(strings.Replace(minimalConfigYAML, "    type: anthropic\n", "    type: openai\n", 1))
	require.NoError(t, err)
}

func cloneFile(file File) File {
	result := File{Providers: make(map[string]Provider, len(file.Providers)), Models: make(map[string]Model, len(file.Models))}
	for key, value := range file.Providers {
		result.Providers[key] = value
	}
	for key, value := range file.Models {
		value.Aliases = append([]string(nil), value.Aliases...)
		result.Models[key] = value
	}
	return result
}

func TestParseModelCatalog_OpenAICompatibleProvider(t *testing.T) {
	yaml := strings.Replace(minimalConfigYAML, "    type: anthropic\n", "    type: openai-compatible\n    baseURL: http://127.0.0.1:11434/v1\n    providerName: ollama\n", 1)
	file, err := ParseModelCatalog(yaml)
	require.NoError(t, err)
	resolved, err := file.ResolveProviderSecrets(func(string) (string, bool) { return "key", true })
	require.NoError(t, err)
	assert.Equal(t, ResolvedProvider{Type: "openai-compatible", APIKey: "key", BaseURL: "http://127.0.0.1:11434/v1", ProviderName: "ollama"}, resolved["anthropic-primary"])
}

func TestParseModelCatalog_SupportsStructuredOutputs(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want ResolvedProvider
	}{
		{
			name: "enabled on openai-compatible",
			yaml: strings.Replace(minimalConfigYAML, "    type: anthropic\n", "    type: openai-compatible\n    baseURL: http://127.0.0.1:11434/v1\n    providerName: ollama\n    supportsStructuredOutputs: true\n", 1),
			want: ResolvedProvider{Type: "openai-compatible", APIKey: "key", BaseURL: "http://127.0.0.1:11434/v1", ProviderName: "ollama", SupportsStructuredOutputs: true},
		},
		{
			name: "explicit false on anthropic",
			yaml: strings.Replace(minimalConfigYAML, "    apiKeyEnv: ANTHROPIC_API_KEY\n", "    apiKeyEnv: ANTHROPIC_API_KEY\n    supportsStructuredOutputs: false\n", 1),
			want: ResolvedProvider{Type: "anthropic", APIKey: "key"},
		},
		{
			name: "explicit false on openai",
			yaml: strings.Replace(strings.Replace(minimalConfigYAML, "type: anthropic", "type: openai", 1), "    apiKeyEnv: ANTHROPIC_API_KEY\n", "    apiKeyEnv: ANTHROPIC_API_KEY\n    supportsStructuredOutputs: false\n", 1),
			want: ResolvedProvider{Type: "openai", APIKey: "key"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := ParseModelCatalog(tc.yaml)
			require.NoError(t, err)
			resolved, err := file.ResolveProviderSecrets(func(string) (string, bool) { return "key", true })
			require.NoError(t, err)
			assert.Equal(t, tc.want, resolved["anthropic-primary"])
		})
	}
}

func TestParseModelCatalog_GrafanaProviderNameIsOrdinary(t *testing.T) {
	yaml := strings.Replace(minimalConfigYAML, "    type: anthropic\n", "    type: openai-compatible\n    baseURL: http://127.0.0.1:11434/v1\n    providerName: grafana\n", 1)
	file, err := ParseModelCatalog(yaml)
	require.NoError(t, err)
	assert.Equal(t, "grafana", file.Providers["anthropic-primary"].ProviderName)
}

func TestParseModelCatalog_ProviderErrorsNameTheField(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{name: "missing base URL", yaml: strings.Replace(minimalConfigYAML, "type: anthropic", "type: openai-compatible", 1), want: "providers.anthropic-primary.baseURL"},
		{name: "provider name on anthropic", yaml: strings.Replace(minimalConfigYAML, "    apiKeyEnv: ANTHROPIC_API_KEY\n", "    apiKeyEnv: ANTHROPIC_API_KEY\n    providerName: other\n", 1), want: "providers.anthropic-primary.providerName"},
		{name: "structured outputs on anthropic", yaml: strings.Replace(minimalConfigYAML, "    apiKeyEnv: ANTHROPIC_API_KEY\n", "    apiKeyEnv: ANTHROPIC_API_KEY\n    supportsStructuredOutputs: true\n", 1), want: "providers.anthropic-primary.supportsStructuredOutputs is only supported for openai-compatible providers"},
		{name: "structured outputs on openai", yaml: strings.Replace(strings.Replace(minimalConfigYAML, "type: anthropic", "type: openai", 1), "    apiKeyEnv: ANTHROPIC_API_KEY\n", "    apiKeyEnv: ANTHROPIC_API_KEY\n    supportsStructuredOutputs: true\n", 1), want: "providers.anthropic-primary.supportsStructuredOutputs is only supported for openai-compatible providers"},
		{name: "unsupported type", yaml: strings.Replace(minimalConfigYAML, "type: anthropic", "type: unsupported", 1), want: `providers.anthropic-primary.type "unsupported" is unsupported (want anthropic, openai or openai-compatible)`},
		{name: "reserved provider name", yaml: fmt.Sprintf(strings.Replace(minimalConfigYAML, "    type: anthropic\n", "    type: openai-compatible\n    baseURL: http://127.0.0.1:11434/v1\n    providerName: %s\n", 1), "gateway"), want: "providers.anthropic-primary.providerName"},
		{name: "reserved provider name with a suffix", yaml: fmt.Sprintf(strings.Replace(minimalConfigYAML, "    type: anthropic\n", "    type: openai-compatible\n    baseURL: http://127.0.0.1:11434/v1\n    providerName: %s\n", 1), "gateway.chat"), want: "providers.anthropic-primary.providerName"},
		{name: "reserved provider name with padding", yaml: fmt.Sprintf(strings.Replace(minimalConfigYAML, "    type: anthropic\n", "    type: openai-compatible\n    baseURL: http://127.0.0.1:11434/v1\n    providerName: %s\n", 1), `" gateway"`), want: "providers.anthropic-primary.providerName"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModelCatalog(tc.yaml)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func testModelConfig() File {
	return File{
		Providers: map[string]Provider{"provider": {Type: "anthropic", APIKeyEnv: "DUMMY_SECRET_REFERENCE"}},
		Models: map[string]Model{
			"canonical": {Name: "Canonical", Primary: Primary{Provider: "provider", Model: "native"}},
		},
	}
}

func TestFile_ConfiguredDiscoveryLimits(t *testing.T) {
	for _, field := range []string{"rows", "public IDs", "aliases", "candidates", "name", "description", "instance", "provider", "model"} {
		t.Run(field, func(t *testing.T) {
			for _, extra := range []int{0, 1} {
				file := testModelConfig()
				model := file.Models["canonical"]
				switch field {
				case "rows":
					file.Models = map[string]Model{}
					for i := range maxModelRows + extra {
						file.Models[fmt.Sprintf("model-%d", i)] = model
					}
				case "public IDs":
					file.Models = map[string]Model{}
					for i := range 8 {
						route := model
						aliases := 127
						if i == 0 {
							aliases += extra
						}
						for j := range aliases {
							route.Aliases = append(route.Aliases, fmt.Sprintf("alias-%d-%03d", i, j))
						}
						file.Models[fmt.Sprintf("route-%d", i)] = route
					}
				case "aliases":
					for i := range maxAliases + extra {
						model.Aliases = append(model.Aliases, fmt.Sprintf("alias-%d", i))
					}
				case "candidates":
					for i := range MaxProviderAttempts - 1 + extra {
						model.Fallback = append(model.Fallback, Primary{Provider: "provider", Model: fmt.Sprintf("native-%d", i)})
					}
				case "name":
					model.Name = strings.Repeat("a", maxStringBytes+extra)
				case "description":
					model.Description = strings.Repeat("a", maxStringBytes+extra)
				case "instance":
					value := file.Providers["provider"]
					model.Primary.Provider = strings.Repeat("a", maxStringBytes+extra)
					file.Providers = map[string]Provider{model.Primary.Provider: value}
				case "provider":
					file.Providers["provider"] = Provider{Type: "openai-compatible", APIKeyEnv: "DUMMY_SECRET_REFERENCE", BaseURL: "https://backend.invalid", ProviderName: strings.Repeat("a", maxStringBytes+extra)}
				case "model":
					model.Primary.Model = strings.Repeat("a", maxStringBytes+extra)
				}
				if field != "rows" && field != "public IDs" {
					file.Models["canonical"] = model
				}
				err := file.Validate()
				if extra == 0 {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					assert.NotContains(t, err.Error(), "DUMMY_SECRET_REFERENCE")
				}
			}
		})
	}
	t.Run("unrelated provider has no discovery string budget", func(t *testing.T) {
		file := testModelConfig()
		file.Providers["unused-account"] = Provider{Type: "openai-compatible", APIKeyEnv: "DUMMY_SECRET_REFERENCE", BaseURL: "https://backend.invalid", ProviderName: strings.Repeat("a", maxStringBytes+1)}
		require.NoError(t, file.Validate())
	})
}

func TestParseModelCatalog_ServerFallbacksPolicy(t *testing.T) {
	const primaryModel = "      model: claude-test\n"
	withPrimaryPolicy := func(policy string) string {
		return strings.Replace(minimalConfigYAML, primaryModel, primaryModel+policy, 1)
	}
	withFallback := strings.Replace(minimalConfigYAML, "models:", "  backup:\n    type: anthropic\n    apiKeyEnv: BACKUP_KEY\nmodels:", 1) +
		"    fallback:\n      - provider: backup\n        model: second\n        serverFallbacks:\n          models: [claude-b]\n"
	for _, tc := range []struct {
		name  string
		yaml  string
		check func(t *testing.T, model Model)
	}{
		{
			name: "primary policy",
			yaml: withPrimaryPolicy("      serverFallbacks:\n        models: [claude-b, claude-c]\n        allowDefault: true\n"),
			check: func(t *testing.T, model Model) {
				assert.Equal(t, &ServerFallbacks{Models: []string{"claude-b", "claude-c"}, AllowDefault: true}, model.Primary.ServerFallbacks)
			},
		},
		{
			name: "default only",
			yaml: withPrimaryPolicy("      serverFallbacks:\n        allowDefault: true\n"),
			check: func(t *testing.T, model Model) {
				assert.Equal(t, &ServerFallbacks{AllowDefault: true}, model.Primary.ServerFallbacks)
			},
		},
		{
			name: "fallback entry policy",
			yaml: withFallback,
			check: func(t *testing.T, model Model) {
				assert.Nil(t, model.Primary.ServerFallbacks)
				require.Len(t, model.Fallback, 1)
				assert.Equal(t, &ServerFallbacks{Models: []string{"claude-b"}}, model.Fallback[0].ServerFallbacks)
			},
		},
		{
			name: "null policy is absent",
			yaml: withPrimaryPolicy("      serverFallbacks: null\n"),
			check: func(t *testing.T, model Model) {
				assert.Nil(t, model.Primary.ServerFallbacks)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := ParseModelCatalog(tc.yaml)
			require.NoError(t, err)
			tc.check(t, file.Models["grafana/assistant"])
		})
	}

	const secretModel = "policy-value-must-not-leak"
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{name: "openai provider", yaml: strings.Replace(withPrimaryPolicy("      serverFallbacks:\n        models: ["+secretModel+"]\n"), "type: anthropic", "type: openai", 1), want: "models.grafana/assistant.primary.serverFallbacks is only supported for anthropic providers"},
		{name: "openai-compatible provider", yaml: strings.Replace(withPrimaryPolicy("      serverFallbacks:\n        models: ["+secretModel+"]\n"), "    type: anthropic\n", "    type: openai-compatible\n    baseURL: http://127.0.0.1:11434/v1\n", 1), want: "models.grafana/assistant.primary.serverFallbacks is only supported for anthropic providers"},
		{name: "openai fallback entry", yaml: strings.Replace(withFallback, "  backup:\n    type: anthropic", "  backup:\n    type: openai", 1), want: "models.grafana/assistant.fallback[0].serverFallbacks is only supported for anthropic providers"},
		{name: "empty policy", yaml: withPrimaryPolicy("      serverFallbacks: {}\n"), want: "models.grafana/assistant.primary.serverFallbacks must allow default or at least one model"},
		{name: "explicit false without models", yaml: withPrimaryPolicy("      serverFallbacks:\n        allowDefault: false\n        models: []\n"), want: "models.grafana/assistant.primary.serverFallbacks must allow default or at least one model"},
		{name: "empty model", yaml: withPrimaryPolicy("      serverFallbacks:\n        models: ['']\n"), want: "models.grafana/assistant.primary.serverFallbacks.models[0] must not be empty"},
		{name: "blank model", yaml: withPrimaryPolicy("      serverFallbacks:\n        models: [claude-b, '  ']\n"), want: "models.grafana/assistant.primary.serverFallbacks.models[1] must not be empty"},
		{name: "overlong model", yaml: withPrimaryPolicy("      serverFallbacks:\n        models: [" + strings.Repeat("m", maxStringBytes+1) + "]\n"), want: "models.grafana/assistant.primary.serverFallbacks.models[0] is invalid or exceeds limit"},
		{name: "repeated model", yaml: withPrimaryPolicy("      serverFallbacks:\n        models: [" + secretModel + ", " + secretModel + "]\n"), want: "models.grafana/assistant.primary.serverFallbacks.models[1] repeats a model"},
		{name: "unknown policy field", yaml: withPrimaryPolicy("      serverFallbacks:\n        models: [claude-b]\n        allowAll: true\n"), want: "config: invalid YAML fields or values"},
		{name: "repeated candidate with another policy", yaml: strings.Replace(withFallback, "      - provider: backup\n        model: second\n", "      - provider: anthropic-primary\n        model: claude-test\n", 1), want: "models.grafana/assistant.fallback[0] repeats a candidate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModelCatalog(tc.yaml)
			require.ErrorContains(t, err, tc.want)
			assert.NotContains(t, err.Error(), secretModel)
			assert.NotContains(t, err.Error(), "BACKUP_KEY")
		})
	}

	t.Run("invalid UTF-8 model", func(t *testing.T) {
		file := testModelConfig()
		model := file.Models["canonical"]
		model.Primary.ServerFallbacks = &ServerFallbacks{Models: []string{"claude-\xff"}}
		file.Models["canonical"] = model
		require.ErrorContains(t, file.Validate(), "models.canonical.primary.serverFallbacks.models[0] is invalid or exceeds limit")
	})
}

func TestParseModelCatalog_ServerFallbacksPerDestination(t *testing.T) {
	document := func(aProvider, aPolicy, bProvider, bPolicy string) string {
		return strings.NewReplacer("A_PROVIDER", aProvider, "A_SKILLS\n", aPolicy, "B_PROVIDER", bProvider, "B_SKILLS\n", bPolicy).Replace(sharedDestinationYAML)
	}
	primaryPolicy := func(body string) string { return "      serverFallbacks:\n" + body }
	fallbackPolicy := func(body string) string { return "        serverFallbacks:\n" + body }
	const differs = "config: models.grafana/b.fallback[0].serverFallbacks differs from models.grafana/a.primary.serverFallbacks for the same provider and model"

	for _, tc := range []struct {
		name    string
		aPolicy string
		bPolicy string
	}{
		{name: "only the first declares a policy", aPolicy: primaryPolicy("        models: [claude-b]\n")},
		{name: "only the second declares a policy", bPolicy: fallbackPolicy("          models: [claude-b]\n")},
		{name: "different models", aPolicy: primaryPolicy("        models: [claude-b]\n"), bPolicy: fallbackPolicy("          models: [claude-c]\n")},
		{name: "different allowDefault", aPolicy: primaryPolicy("        models: [claude-b]\n"), bPolicy: fallbackPolicy("          models: [claude-b]\n          allowDefault: true\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModelCatalog(document("anthropic-main", tc.aPolicy, "anthropic-main", tc.bPolicy))
			require.EqualError(t, err, differs)
			assert.NotContains(t, err.Error(), "claude-b")
		})
	}

	t.Run("equal policies in a different order load", func(t *testing.T) {
		_, err := ParseModelCatalog(document("anthropic-main", primaryPolicy("        models: [claude-b, claude-c]\n"), "anthropic-main", fallbackPolicy("          models: [claude-c, claude-b]\n")))
		require.NoError(t, err)
	})

	t.Run("second provider instance may differ", func(t *testing.T) {
		_, err := ParseModelCatalog(document("anthropic-main", "", "anthropic-skills", fallbackPolicy("          allowDefault: true\n")))
		require.NoError(t, err)
	})
}
