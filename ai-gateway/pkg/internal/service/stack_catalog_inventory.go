package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode"
)

const (
	maxListedModelIDs   = 512
	maxListedModelBytes = 256
)

// ErrCatalogProviderNotFound reports that the checked catalog has no provider by that name.
var ErrCatalogProviderNotFound = errors.New("gateway service: catalog provider not found")

// ListingStatus is a closed outcome of a provider inventory request.
type ListingStatus string

const (
	ListingOK          ListingStatus = ListingStatus(HealthOK)
	ListingPartial     ListingStatus = "partial"
	ListingInvalidKey  ListingStatus = ListingStatus(HealthInvalidKey)
	ListingUnreachable ListingStatus = ListingStatus(HealthUnreachable)
	ListingRefused     ListingStatus = ListingStatus(HealthRefused)
	ListingNotChecked  ListingStatus = ListingStatus(HealthNotChecked)
)

// CatalogProviderModels contains only the listing outcome and safe model IDs.
type CatalogProviderModels struct {
	Status   ListingStatus `json:"status"`
	ModelIDs []string      `json:"modelIds"`
}

// AvailableModels lists one named provider from the checked saved catalog, without caching.
func (c *CatalogHealthChecker) AvailableModels(ctx context.Context, namespace string, stack StackCatalog, provider string) (CatalogProviderModels, error) {
	file, keys, err := parseStackCatalog(stack)
	if err != nil {
		return CatalogProviderModels{}, err
	}
	configured, ok := file.Providers[provider]
	if !ok {
		return CatalogProviderModels{}, ErrCatalogProviderNotFound
	}
	id := healthCatalogID{namespace, stack.Name}
	if !c.admit(id) {
		return CatalogProviderModels{}, ErrCatalogHealthBusy
	}
	defer c.release(id)

	result := CatalogProviderModels{ModelIDs: []string{}}
	seen := make(map[string]struct{})
	omitted := false
	status, complete := c.walkListing(ctx, resolveStackProvider(configured, stack, provider), func(page listingPage) bool {
		for _, entry := range page.Data {
			id := entry.ID
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			if len(seen) == maxListedModelIDs || !safeListedID(id, keys) {
				omitted = true
				continue
			}
			seen[id] = struct{}{}
			result.ModelIDs = append(result.ModelIDs, id)
		}
		return len(seen) < maxListedModelIDs
	})
	result.Status = ListingStatus(status)
	if result.Status == ListingOK && (!complete || omitted) {
		result.Status = ListingPartial
	}
	slices.Sort(result.ModelIDs)
	return result, nil
}

func safeListedID(id string, keys []string) bool {
	return len(id) <= maxListedModelBytes && !strings.ContainsFunc(id, unicode.IsControl) &&
		!slices.ContainsFunc(keys, func(key string) bool { return strings.Contains(id, key) })
}
