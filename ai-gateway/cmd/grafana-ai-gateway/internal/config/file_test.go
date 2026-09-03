package config

import (
	"os"
	"path/filepath"
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

func TestLoadFile_StrictBoundedDocument(t *testing.T) {
	t.Run("valid below and exact limit", func(t *testing.T) {
		path := writeConfigFile(t, minimalConfigYAML)
		for _, limit := range []int64{int64(len(minimalConfigYAML) + 1), int64(len(minimalConfigYAML))} {
			file, err := LoadFile(path, limit)
			require.NoError(t, err)
			require.Len(t, file.Models, 1)
			assert.Equal(t, []string{"assistant"}, file.Models["grafana/assistant"].Aliases)
		}
	})

	t.Run("over limit", func(t *testing.T) {
		_, err := LoadFile(writeConfigFile(t, minimalConfigYAML), int64(len(minimalConfigYAML)-1))
		require.Error(t, err)
	})

	t.Run("regular file required", func(t *testing.T) {
		_, err := LoadFile(t.TempDir(), 1024)
		require.Error(t, err)
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
		{name: "unknown provider type", yaml: strings.Replace(minimalConfigYAML, "type: anthropic", "type: openai", 1)},
		{name: "missing api key reference", yaml: strings.Replace(minimalConfigYAML, "    apiKeyEnv: ANTHROPIC_API_KEY\n", "", 1)},
		{name: "missing model name", yaml: strings.Replace(minimalConfigYAML, "    name: Grafana Assistant\n", "", 1)},
		{name: "unknown provider reference", yaml: strings.Replace(minimalConfigYAML, "provider: anthropic-primary", "provider: missing", 1)},
		{name: "missing backend model", yaml: strings.Replace(minimalConfigYAML, "      model: claude-test\n", "", 1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadFile(writeConfigFile(t, tc.yaml), 1<<20)
			require.Error(t, err)
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
	base := File{
		Providers: map[string]Provider{"provider": {Type: "anthropic", APIKeyEnv: "KEY"}},
		Models: map[string]Model{
			"canonical": {Name: "Canonical", Primary: Primary{Provider: "provider", Model: "backend"}},
		},
	}
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

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "models.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
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
