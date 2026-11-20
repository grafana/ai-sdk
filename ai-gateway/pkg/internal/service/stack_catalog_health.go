package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/nativemodel"
)

const (
	healthMaxRuns         = 4
	healthMaxProviders    = 8
	healthProviderWorkers = 4
	healthRunTimeout      = 8 * time.Second
	healthProviderTimeout = 3 * time.Second
	healthResponseBytes   = 512 << 10
	healthAnthropicPages  = 3
	healthAnthropicLimit  = "100"
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
	mu        sync.Mutex
	active    map[healthCatalogID]struct{}
}

// NewCatalogHealthChecker creates the checker shared by the Gateway's health requests.
func NewCatalogHealthChecker(transport http.RoundTripper) *CatalogHealthChecker {
	return &CatalogHealthChecker{transport: transport, active: make(map[healthCatalogID]struct{})}
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
	file, _, err := parseStackCatalog(stack)
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

	results := make([]providerHealth, min(len(names), healthMaxProviders))
	var workers sync.WaitGroup
	for worker := range min(healthProviderWorkers, len(results)) {
		workers.Go(func() {
			for i := worker; i < len(results) && ctx.Err() == nil; i += healthProviderWorkers {
				name := names[i]
				results[i] = c.list(ctx, resolveStackProvider(file.Providers[name], stack, name), targets[name])
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

type listingPage struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	HasMore *bool  `json:"has_more"`
	LastID  string `json:"last_id"`
}

func (c *CatalogHealthChecker) list(ctx context.Context, p config.ResolvedProvider, targets map[string]struct{}) providerHealth {
	ctx, cancel := context.WithTimeout(ctx, healthProviderTimeout)
	defer cancel()

	base, path, pages := p.BaseURL, "models", 1
	custom := base != ""
	switch p.Type {
	case "anthropic":
		path, pages = "v1/models", healthAnthropicPages
		if !custom {
			base = nativemodel.AnthropicBaseURL
		}
	case "openai":
		if !custom {
			base = nativemodel.OpenAIBaseURL
		}
	case "openai-compatible":
		if !custom {
			return providerHealth{status: HealthRefused}
		}
	default:
		return providerHealth{status: HealthRefused}
	}
	endpoint, err := url.JoinPath(base, path)
	if err != nil {
		return providerHealth{status: HealthRefused}
	}

	health := providerHealth{found: make(map[string]struct{})}
	seen := make(map[string]struct{}, pages)
	var cursor string
	for range pages {
		page, status := c.listPage(ctx, p, endpoint, cursor, custom)
		if status != HealthOK {
			if health.status == "" {
				health.status = status
			}
			return health
		}
		health.status = HealthOK
		for _, entry := range page.Data {
			if _, wanted := targets[entry.ID]; wanted {
				health.found[entry.ID] = struct{}{}
			}
		}
		if page.HasMore == nil || !*page.HasMore {
			health.complete = !custom
			return health
		}
		if len(health.found) == len(targets) || page.LastID == "" {
			return health
		}
		if _, repeated := seen[page.LastID]; repeated {
			return health
		}
		cursor = page.LastID
		seen[cursor] = struct{}{}
	}
	return health
}

func (c *CatalogHealthChecker) listPage(ctx context.Context, p config.ResolvedProvider, endpoint, cursor string, custom bool) (listingPage, HealthStatus) {
	var page listingPage
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return page, HealthRefused
	}
	req.Header.Set("Accept", "application/json")
	if p.Type == "anthropic" {
		req.Header.Set("x-api-key", p.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		query := url.Values{"limit": {healthAnthropicLimit}}
		if cursor != "" {
			query.Set("after_id", cursor)
		}
		req.URL.RawQuery = query.Encode()
	} else {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := c.transport.RoundTrip(req)
	if err != nil {
		return page, HealthUnreachable
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return page, HealthInvalidKey
	case custom && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented):
		return page, HealthNotChecked
	case resp.StatusCode >= http.StatusInternalServerError:
		return page, HealthUnreachable
	case resp.StatusCode != http.StatusOK:
		return page, HealthRefused
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, healthResponseBytes+1))
	if err != nil {
		return page, HealthUnreachable
	}
	if len(body) > healthResponseBytes {
		return page, HealthNotChecked
	}
	if json.Unmarshal(body, &page) != nil || page.Data == nil || (p.Type == "anthropic" && page.HasMore == nil) {
		return page, HealthRefused
	}
	for _, entry := range page.Data {
		if entry.ID == "" {
			return page, HealthRefused
		}
	}
	return page, HealthOK
}
