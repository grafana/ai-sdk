package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func discoveryConfig() File {
	return File{Providers: map[string]Provider{"instance": {Type: "anthropic", APIKeyEnv: "DUMMY_SECRET_REFERENCE"}}, Models: map[string]Model{"public": {Name: "Model", Primary: Primary{Provider: "instance", Model: "native"}}}}
}

func TestFile_ConfiguredDiscoveryLimits(t *testing.T) {
	for _, field := range []string{"rows", "aliases", "candidates", "name", "description", "instance", "provider", "model"} {
		t.Run(field, func(t *testing.T) {
			for _, extra := range []int{0, 1} {
				file := discoveryConfig()
				model := file.Models["public"]
				switch field {
				case "rows":
					file.Models = map[string]Model{}
					for i := range discovery.MaxModelRows + extra {
						file.Models[fmt.Sprintf("model-%d", i)] = model
					}
				case "aliases":
					for i := range discovery.MaxAliases + extra {
						model.Aliases = append(model.Aliases, fmt.Sprintf("alias-%d", i))
					}
				case "candidates":
					for i := range discovery.MaxCandidates - 1 + extra {
						model.Fallback = append(model.Fallback, Primary{Provider: "instance", Model: fmt.Sprintf("native-%d", i)})
					}
				case "name":
					model.Name = strings.Repeat("a", discovery.MaxStringBytes+extra)
				case "description":
					model.Description = strings.Repeat("a", discovery.MaxStringBytes+extra)
				case "instance":
					value := file.Providers["instance"]
					model.Primary.Provider = strings.Repeat("a", discovery.MaxStringBytes+extra)
					file.Providers = map[string]Provider{model.Primary.Provider: value}
				case "provider":
					file.Providers["instance"] = Provider{Type: "openai-compatible", APIKeyEnv: "DUMMY_SECRET_REFERENCE", BaseURL: "https://backend.invalid", ProviderName: strings.Repeat("a", discovery.MaxStringBytes+extra)}
				case "model":
					model.Primary.Model = strings.Repeat("a", discovery.MaxStringBytes+extra)
				}
				if field != "rows" {
					file.Models["public"] = model
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
}

func TestFile_AggregateAliasExpansionLimit(t *testing.T) {
	for _, extra := range []int{0, 1} {
		t.Run(fmt.Sprintf("extra-%d", extra), func(t *testing.T) {
			file := discoveryConfig()
			model := file.Models["public"]
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
			err := file.Validate()
			if extra == 0 {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "DUMMY_SECRET_REFERENCE")
			}
		})
	}
}

func TestFile_UnrelatedProviderHasNoDiscoveryStringBudget(t *testing.T) {
	file := discoveryConfig()
	file.Providers["unused-account"] = Provider{Type: "openai-compatible", APIKeyEnv: "DUMMY_SECRET_REFERENCE", BaseURL: "https://backend.invalid", ProviderName: strings.Repeat("a", discovery.MaxStringBytes+1)}
	require.NoError(t, file.Validate())
}

func TestDiscovery_DefaultsRemainIndependent(t *testing.T) {
	settings, err := ParseSettings(nil, func(name string) (string, bool) { value, ok := baseSettingsEnvironment()[name]; return value, ok })
	require.NoError(t, err)
	assert.EqualValues(t, 1<<20, settings.DiscoveryResponseBytes)
}
