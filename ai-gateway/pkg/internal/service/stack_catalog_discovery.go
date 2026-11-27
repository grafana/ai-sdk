package service

import (
	"context"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
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

func newCatalogDiscoveryListings(transport http.RoundTripper) *catalogDiscoveryListings {
	return &catalogDiscoveryListings{
		checker: NewCatalogHealthChecker(transport, outbound.CustomEndpoints{}),
		entries: make(map[discoveryInventoryKey]discoveryInventoryEntry),
	}
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

func (c *catalogDiscoveryListings) appendUnlistedModels(
	ctx context.Context, key discoveryInventoryKey, file config.File, keys []string,
	resolveProvider func(string) config.ResolvedProvider, infos []catalog.ModelInfo,
) []catalog.ModelInfo {
	if len(file.Providers) == 0 {
		return infos
	}
	listed := make(map[string]struct{}, len(infos))
	for _, info := range infos {
		listed[info.ID] = struct{}{}
		for _, alias := range info.Aliases {
			listed[alias] = struct{}{}
		}
	}
	remaining := config.MaxDiscoveryModelIDs - len(listed)
	if remaining <= 0 {
		return infos
	}
	containsKey := listingKeyMatcher(keys)
	names := make([]string, 0, healthMaxProviders)
	for _, name := range slices.Sorted(maps.Keys(file.Providers)) {
		p := file.Providers[name]
		if p.ServeAllModels && !containsKey(name) && !containsKey(providerSlug(p.Type, p.ProviderName)) {
			names = append(names, name)
			if len(names) == healthMaxProviders {
				break
			}
		}
	}

	inventories := c.inventories(ctx, key, names, resolveProvider, containsKey)
	for i, inventory := range inventories {
		for _, model := range inventory {
			selector := names[i] + "/" + model
			if _, shadowed := listed[selector]; shadowed || containsKey(selector) {
				continue
			}
			candidate, allowed := unlistedCandidate(file, names[i], model)
			if !allowed {
				continue
			}
			infos = append(infos, catalog.ModelInfo{ID: selector, Name: model, Candidates: []catalog.ConfiguredCandidate{candidate}})
			remaining--
			if remaining == 0 {
				return infos
			}
		}
	}
	return infos
}

func (c *catalogDiscoveryListings) inventories(
	ctx context.Context, key discoveryInventoryKey, names []string,
	resolveProvider func(string) config.ResolvedProvider, containsKey func(string) bool,
) [][]string {
	inventories := make([][]string, len(names))
	var missing []int
	for i, name := range names {
		providerKey := key
		providerKey.provider = name
		if key.revision != "" {
			if ids, cached := c.cached(providerKey); cached {
				inventories[i] = ids
				continue
			}
		}
		missing = append(missing, i)
	}
	if len(missing) == 0 || !c.checker.admit(key.catalog) {
		return inventories
	}
	defer c.checker.release(key.catalog)
	ctx, cancel := context.WithTimeout(ctx, discoveryListingTimeout)
	defer cancel()

	var workers sync.WaitGroup
	for worker := range min(healthProviderWorkers, len(missing)) {
		workers.Go(func() {
			for offset := worker; offset < len(missing) && ctx.Err() == nil; offset += healthProviderWorkers {
				i := missing[offset]
				providerKey := key
				providerKey.provider = names[i]
				inventories[i] = c.inventoryIDs(ctx, providerKey, resolveProvider(names[i]), containsKey)
			}
		})
	}
	workers.Wait()
	return inventories
}
