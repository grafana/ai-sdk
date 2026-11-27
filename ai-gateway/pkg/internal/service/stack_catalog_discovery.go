package service

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
)

const discoveryListingTimeout = 2 * time.Second

func (c *CatalogHealthChecker) appendUnlistedModels(ctx context.Context, stack StackCatalog, index stackIndex, keys []string, infos []catalog.ModelInfo) []catalog.ModelInfo {
	remaining := config.MaxDiscoveryModelIDs - len(index.canonical)
	if remaining <= 0 {
		return infos
	}
	containsKey := listingKeyMatcher(keys)
	names := make([]string, 0, healthMaxProviders)
	for _, name := range slices.Sorted(maps.Keys(index.file.Providers)) {
		provider := index.file.Providers[name]
		if index.file.ServesUnlistedModels(name) && !containsKey(name) && !containsKey(providerSlug(provider.Type, provider.ProviderName)) {
			names = append(names, name)
			if len(names) == healthMaxProviders {
				break
			}
		}
	}
	if len(names) == 0 {
		return infos
	}
	caller, ok := gatewayauth.CallerFromContext(ctx)
	id := healthCatalogID{caller.Namespace, stack.Name}
	if !ok || !c.admit(id) {
		return infos
	}
	defer c.release(id)
	ctx, cancel := context.WithTimeout(ctx, discoveryListingTimeout)
	defer cancel()

	inventories := make([]CatalogProviderModels, len(names))
	var workers sync.WaitGroup
	for worker := range min(healthProviderWorkers, len(names)) {
		workers.Go(func() {
			for i := worker; i < len(names) && ctx.Err() == nil; i += healthProviderWorkers {
				name := names[i]
				inventories[i] = c.inventory(ctx, resolveStackProvider(index.file.Providers[name], stack, name), containsKey)
			}
		})
	}
	workers.Wait()

	for i, inventory := range inventories {
		for _, model := range inventory.ModelIDs {
			selector := names[i] + "/" + model
			if _, listed := index.canonical[selector]; listed || containsKey(selector) {
				continue
			}
			group, err := index.resolve(selector)
			if err != nil {
				continue
			}
			infos = append(infos, catalog.ModelInfo{ID: selector, Name: model, Candidates: group.candidates})
			remaining--
			if remaining == 0 {
				return infos
			}
		}
	}
	return infos
}
