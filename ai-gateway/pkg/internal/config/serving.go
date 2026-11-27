package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
)

const maxBlockedModelIDs = 128

// AllowsUnlistedModel reports whether the named provider serves the exact backend ID.
func (file File) AllowsUnlistedModel(instance, modelID string) bool {
	provider := file.Providers[instance]
	return provider.ServeAllModels && !slices.Contains(provider.ModelBlocklist, modelID) &&
		ValidatePublicID(modelID) == nil && ValidatePublicID(instance+"/"+modelID) == nil
}

func (file File) validateServing(operator bool) error {
	prefix := "modelcatalog"
	if operator {
		prefix = "config"
	}
	for _, name := range slices.Sorted(maps.Keys(file.Providers)) {
		provider := file.Providers[name]
		if operator && provider.ServeAllModels {
			if ValidatePublicID(name) != nil || strings.Contains(name, "/") {
				return fmt.Errorf("config: providers.%s.serveAllModels requires a valid instance name without a slash", name)
			}
			if !validDiscoveryText(provider.ProviderName) {
				return fmt.Errorf("config: providers.%s.providerName is invalid or exceeds the discovery limit", name)
			}
		}
		ids := provider.ModelBlocklist
		if len(ids) > maxBlockedModelIDs {
			return fmt.Errorf("%s: providers.%s.modelBlocklist allows at most %d entries", prefix, name, maxBlockedModelIDs)
		}
		for index, id := range ids {
			if err := ValidatePublicID(id); err != nil {
				return fmt.Errorf("%s: %s: %w", prefix, blocklistPath(name, index), err)
			}
			if slices.Contains(ids[:index], id) {
				return fmt.Errorf("%s: %s repeats an earlier entry", prefix, blocklistPath(name, index))
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(file.Models)) {
		model := file.Models[id]
		for index, candidate := range append([]Primary{model.Primary}, model.Fallback...) {
			ids := file.Providers[candidate.Provider].ModelBlocklist
			if len(ids) == 0 {
				continue
			}
			if blocked := slices.Index(ids, candidate.Model); blocked >= 0 {
				return fmt.Errorf("%s: %s conflicts with a listed target", prefix, blocklistPath(candidate.Provider, blocked))
			}
			if candidate.ServerFallbacks == nil {
				continue
			}
			path := fmt.Sprintf("models.%s.primary", id)
			if index > 0 {
				path = fmt.Sprintf("models.%s.fallback[%d]", id, index-1)
			}
			if candidate.ServerFallbacks.AllowDefault {
				return fmt.Errorf("config: %s.serverFallbacks.allowDefault is incompatible with providers.%s.modelBlocklist", path, candidate.Provider)
			}
			for i, fallback := range candidate.ServerFallbacks.Models {
				if blocked := slices.Index(ids, fallback); blocked >= 0 {
					return fmt.Errorf("config: %s.serverFallbacks.models[%d] conflicts with %s", path, i, blocklistPath(candidate.Provider, blocked))
				}
			}
		}
	}
	return nil
}

func blocklistPath(name string, index int) string {
	return fmt.Sprintf("providers.%s.modelBlocklist[%d]", name, index)
}

func validateServingYAML(providers *yaml.Node) error {
	if providers.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(providers.Content); i += 2 {
		name, provider := providers.Content[i].Value, providers.Content[i+1]
		if provider.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j < len(provider.Content); j += 2 {
			field, value := provider.Content[j].Value, provider.Content[j+1]
			switch field {
			case "serveAllModels":
				if value.Tag != "!!bool" {
					return fmt.Errorf("config: providers.%s.serveAllModels must be a boolean", name)
				}
			case "modelBlocklist":
				if value.Kind != yaml.SequenceNode {
					return fmt.Errorf("config: providers.%s.modelBlocklist must be a sequence", name)
				}
				for index, entry := range value.Content {
					if entry.Tag != "!!str" {
						return fmt.Errorf("config: %s must be a string", blocklistPath(name, index))
					}
				}
			}
		}
	}
	return nil
}
