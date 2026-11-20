package service

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
)

const (
	healthMaxRuns         = 4
	healthMaxProviders    = 8
	healthProviderWorkers = 4
	healthRunTimeout      = 8 * time.Second
)

// ErrCatalogHealthBusy reports that a health check could not be admitted.
var ErrCatalogHealthBusy = errors.New("gateway service: catalog health check busy")

// HealthStatus is a closed outcome of a provider or model listing check.
type HealthStatus string

const (
	HealthOK           HealthStatus = "ok"
	HealthInvalidKey   HealthStatus = "invalidKey"
	HealthUnknownModel HealthStatus = "unknownModel"
	HealthUnreachable  HealthStatus = "unreachable"
	HealthRefused      HealthStatus = "refused"
	HealthNotChecked   HealthStatus = "notChecked"
)

// ModelHealth reports listing outcomes in catalog target order.
type ModelHealth struct {
	Primary  HealthStatus   `json:"primary"`
	Fallback []HealthStatus `json:"fallback"`
}

// CatalogHealth contains only configured names and fixed listing outcomes.
type CatalogHealth struct {
	Providers map[string]HealthStatus `json:"providers"`
	Models    map[string]ModelHealth  `json:"models"`
}

type healthCatalogID struct{ namespace, name string }

// CatalogHealthChecker runs bounded model-listing checks.
type CatalogHealthChecker struct {
	transport http.RoundTripper
	endpoints outbound.CustomEndpoints
	mu        sync.Mutex
	active    map[healthCatalogID]struct{}
}

// NewCatalogHealthChecker creates the checker shared by the Gateway's health
// requests. A catalog whose custom endpoints aren't all in endpoints is
// refused before any listing request.
func NewCatalogHealthChecker(transport http.RoundTripper, endpoints outbound.CustomEndpoints) *CatalogHealthChecker {
	return &CatalogHealthChecker{transport: transport, endpoints: endpoints, active: make(map[healthCatalogID]struct{})}
}

// Check uses only the checked stored catalog and its decrypted keys.
func (c *CatalogHealthChecker) Check(ctx context.Context, namespace string, stack StackCatalog) (CatalogHealth, error) {
	id := healthCatalogID{namespace, stack.Name}
	if !c.admit(id) {
		return CatalogHealth{}, ErrCatalogHealthBusy
	}
	defer c.release(id)
	ctx, cancel := context.WithTimeout(ctx, healthRunTimeout)
	defer cancel()
	file, keys, err := parseStackCatalog(stack, c.endpoints)
	if err != nil {
		return CatalogHealth{}, err
	}

	names := slices.Sorted(maps.Keys(file.Providers))
	targets := make(map[string]map[string]struct{}, len(names))
	for _, name := range names {
		targets[name] = make(map[string]struct{})
	}
	for _, model := range file.Models {
		targets[model.Primary.Provider][model.Primary.Model] = struct{}{}
		for _, fallback := range model.Fallback {
			targets[fallback.Provider][fallback.Model] = struct{}{}
		}
	}

	containsKey := listingKeyMatcher(keys)
	results := make([]providerHealth, min(len(names), healthMaxProviders))
	var workers sync.WaitGroup
	for worker := range min(healthProviderWorkers, len(results)) {
		workers.Go(func() {
			for i := worker; i < len(results) && ctx.Err() == nil; i += healthProviderWorkers {
				name := names[i]
				results[i] = c.list(ctx, resolveStackProvider(file.Providers[name], stack, name), targets[name], containsKey)
			}
		})
	}
	workers.Wait()

	providers := make(map[string]HealthStatus, len(names))
	byName := make(map[string]providerHealth, len(results))
	for i, name := range names {
		providers[name] = HealthNotChecked
		if i < len(results) && results[i].status != "" {
			byName[name] = results[i]
			providers[name] = results[i].status
		}
	}
	models := make(map[string]ModelHealth, len(file.Models))
	for id, model := range file.Models {
		fallback := make([]HealthStatus, len(model.Fallback))
		for i, target := range model.Fallback {
			fallback[i] = byName[target.Provider].model(target.Model)
		}
		models[id] = ModelHealth{Primary: byName[model.Primary.Provider].model(model.Primary.Model), Fallback: fallback}
	}
	return CatalogHealth{Providers: providers, Models: models}, nil
}

func (c *CatalogHealthChecker) admit(id healthCatalogID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.active[id]; exists || len(c.active) >= healthMaxRuns {
		return false
	}
	c.active[id] = struct{}{}
	return true
}

func (c *CatalogHealthChecker) release(id healthCatalogID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.active, id)
}

type providerHealth struct {
	status   HealthStatus
	found    map[string]struct{}
	complete bool
}

func (health providerHealth) model(id string) HealthStatus {
	if _, found := health.found[id]; found {
		return HealthOK
	}
	if health.complete {
		return HealthUnknownModel
	}
	return HealthNotChecked
}

func (c *CatalogHealthChecker) list(ctx context.Context, p config.ResolvedProvider, targets map[string]struct{}, containsKey func(string) bool) providerHealth {
	found := make(map[string]struct{})
	status, complete := c.walkListing(ctx, p, containsKey, func(page listingPage) bool {
		for _, entry := range page.Data {
			if _, wanted := targets[entry.ID]; wanted {
				found[entry.ID] = struct{}{}
			}
		}
		return len(found) != len(targets)
	})
	return providerHealth{status: status, found: found, complete: complete && p.BaseURL == ""}
}
