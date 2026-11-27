package stackcatalog

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
)

const (
	maxBlockedModelIDs = 128
	maxBackendIDBytes  = 2048
)

type servingPolicy struct {
	all     bool
	blocked map[string]int
}

// ServesUnlistedModels reports whether the provider opted in.
func (c Catalog) ServesUnlistedModels(instance string) bool { return c.serving[instance].all }

// AllowsUnlistedModel reports whether the named provider serves the exact backend ID.
func (c Catalog) AllowsUnlistedModel(instance, modelID string) bool {
	policy := c.serving[instance]
	_, blocked := policy.blocked[modelID]
	return policy.all && !blocked && validBackendID(modelID)
}

func validBackendID(id string) bool {
	return len(id) > 0 && len(id) <= maxBackendIDBytes && utf8.ValidString(id) &&
		strings.TrimSpace(id) == id && !strings.ContainsFunc(id, unicode.IsControl)
}

func parseServingPolicies(spec v1alpha1.Spec, file config.File) (map[string]servingPolicy, error) {
	policies := make(map[string]servingPolicy)
	for _, name := range slices.Sorted(maps.Keys(spec.Providers)) {
		provider := spec.Providers[name]
		all := provider.ServeAllModels != nil && *provider.ServeAllModels
		if !all && len(provider.ModelBlocklist) == 0 {
			continue
		}
		if len(provider.ModelBlocklist) > maxBlockedModelIDs {
			return nil, fmt.Errorf("modelcatalog: providers.%s.modelBlocklist allows at most %d entries", name, maxBlockedModelIDs)
		}
		blocked := make(map[string]int, len(provider.ModelBlocklist))
		for index, id := range provider.ModelBlocklist {
			if !validBackendID(id) {
				return nil, fmt.Errorf("modelcatalog: %s must be 1-%d UTF-8 bytes without control characters or leading or trailing whitespace", blocklistPath(name, index), maxBackendIDBytes)
			}
			if _, exists := blocked[id]; exists {
				return nil, fmt.Errorf("modelcatalog: %s repeats an earlier entry", blocklistPath(name, index))
			}
			blocked[id] = index
		}
		policies[name] = servingPolicy{all: all, blocked: blocked}
	}
	conflict := func(target config.Primary) error {
		if index, exists := policies[target.Provider].blocked[target.Model]; exists {
			return fmt.Errorf("modelcatalog: %s conflicts with a listed target", blocklistPath(target.Provider, index))
		}
		return nil
	}
	for _, id := range slices.Sorted(maps.Keys(file.Models)) {
		model := file.Models[id]
		if err := conflict(model.Primary); err != nil {
			return nil, err
		}
		for _, target := range model.Fallback {
			if err := conflict(target); err != nil {
				return nil, err
			}
		}
	}
	return policies, nil
}

func blocklistPath(name string, index int) string {
	return fmt.Sprintf("providers.%s.modelBlocklist[%d]", name, index)
}
