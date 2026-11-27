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

const (
	discoveryListingTimeout = 2 * time.Second
	discoveryInventoryTTL   = 30 * time.Second
	discoveryMaxInventories = 128
)

type discoveryInventoryKey struct {
	catalog            healthCatalogID
	revision, provider string
}

type discoveryInventoryEntry struct {
	modelIDs  []string
	expiresAt time.Time
}

type catalogDiscoveryListings struct {
	checker *CatalogHealthChecker
	mu      sync.Mutex
	entries map[discoveryInventoryKey]discoveryInventoryEntry
}

func (c *catalogDiscoveryListings) cached(key discoveryInventoryKey) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || !time.Now().Before(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return entry.modelIDs, true
}

func (c *catalogDiscoveryListings) remember(key discoveryInventoryKey, modelIDs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	var oldest discoveryInventoryKey
	var firstExpiry time.Time
	for k, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, k)
		} else if firstExpiry.IsZero() || entry.expiresAt.Before(firstExpiry) {
			oldest, firstExpiry = k, entry.expiresAt
		}
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= discoveryMaxInventories {
		delete(c.entries, oldest)
	}
	c.entries[key] = discoveryInventoryEntry{modelIDs: modelIDs, expiresAt: now.Add(discoveryInventoryTTL)}
}

func (c *catalogDiscoveryListings) inventoryIDs(ctx context.Context, key discoveryInventoryKey, provider config.ResolvedProvider, containsKey func(string) bool) []string {
	if key.revision != "" {
		if modelIDs, ok := c.cached(key); ok {
			return modelIDs
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	result, cacheable := c.checker.inventory(ctx, provider, containsKey)
	if key.revision != "" && cacheable {
		c.remember(key, result.ModelIDs)
	}
	return result.ModelIDs
}

func (c *catalogDiscoveryListings) appendUnlistedModels(ctx context.Context, stack StackCatalog, index stackIndex, keys []string, infos []catalog.ModelInfo) []catalog.ModelInfo {
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
	if !ok || !c.checker.admit(id) {
		return infos
	}
	defer c.checker.release(id)
	ctx, cancel := context.WithTimeout(ctx, discoveryListingTimeout)
	defer cancel()

	inventories := make([][]string, len(names))
	var workers sync.WaitGroup
	for worker := range min(healthProviderWorkers, len(names)) {
		workers.Go(func() {
			for i := worker; i < len(names); i += healthProviderWorkers {
				name := names[i]
				key := discoveryInventoryKey{catalog: id, revision: stack.ResourceVersion, provider: name}
				inventories[i] = c.inventoryIDs(ctx, key, resolveStackProvider(index.file.Providers[name], stack, name), containsKey)
			}
		})
	}
	workers.Wait()

	for i, inventory := range inventories {
		for _, model := range inventory {
			selector := names[i] + "/" + model
			if _, listed := index.canonical[selector]; listed || containsKey(selector) || config.ValidatePublicID(selector) != nil {
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
