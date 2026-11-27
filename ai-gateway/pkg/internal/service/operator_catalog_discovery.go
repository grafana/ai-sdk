package service

import (
	"context"
	"net/http"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
)

type operatorCatalogLister struct {
	*Catalog
	listings *catalogDiscoveryListings
}

// DiscoveryLister returns a lister for the startup catalog with its own admission and cache.
func (c *Catalog) DiscoveryLister(transport http.RoundTripper) catalog.ModelLister {
	if len(c.accounts) == 0 {
		return c
	}
	return operatorCatalogLister{Catalog: c, listings: newCatalogDiscoveryListings(transport)}
}

func (lister operatorCatalogLister) ListModels(ctx context.Context) ([]catalog.ModelInfo, error) {
	infos, err := lister.Catalog.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	key := discoveryInventoryKey{revision: "startup"}
	return lister.listings.appendUnlistedModels(ctx, key, lister.unlisted, lister.keys, func(name string) config.ResolvedProvider {
		return lister.accounts[name]
	}, infos), nil
}
