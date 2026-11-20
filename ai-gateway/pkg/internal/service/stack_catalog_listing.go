package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/nativemodel"
)

const (
	listingProviderTimeout = 3 * time.Second
	listingResponseBytes   = 512 << 10
	listingAnthropicPages  = 3
	listingAnthropicLimit  = "100"
	listingCursorBytes     = 1 << 10
)

type listingPage struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	HasMore *bool  `json:"has_more"`
	LastID  string `json:"last_id"`
}

func listingKeyMatcher(keys []string) func(string) bool {
	forms := make([]string, 0, len(keys))
	for _, key := range keys {
		if key != "" {
			forms = append(forms, key)
		}
		if wire := textproto.TrimString(key); wire != "" && wire != key {
			forms = append(forms, wire)
		}
	}
	return func(value string) bool {
		return slices.ContainsFunc(forms, func(key string) bool { return strings.Contains(value, key) })
	}
}

func (c *CatalogHealthChecker) walkListing(ctx context.Context, p config.ResolvedProvider, containsKey func(string) bool, visit func(listingPage) bool) (status HealthStatus, complete bool) {
	ctx, cancel := context.WithTimeout(ctx, listingProviderTimeout)
	defer cancel()

	base, path, pages := p.BaseURL, "models", 1
	custom := base != ""
	switch p.Type {
	case "anthropic":
		path, pages = "v1/models", listingAnthropicPages
		if !custom {
			base = nativemodel.AnthropicBaseURL
		}
	case "openai":
		if !custom {
			base = nativemodel.OpenAIBaseURL
		}
	case "openai-compatible":
		if !custom {
			return HealthRefused, false
		}
	default:
		return HealthRefused, false
	}
	endpoint, err := url.JoinPath(base, path)
	if err != nil {
		return HealthRefused, false
	}

	seen := make(map[string]struct{}, pages)
	var cursor string
	for range pages {
		page, outcome := c.listPage(ctx, p, endpoint, cursor, custom)
		if outcome != HealthOK {
			if status == "" {
				return outcome, false
			}
			return status, false
		}
		status = HealthOK
		more := visit(page)
		if page.HasMore == nil || !*page.HasMore {
			return status, true
		}
		if !more || p.Type != "anthropic" || page.LastID == "" || len(page.LastID) > listingCursorBytes || containsKey(page.LastID) {
			return status, false
		}
		if _, repeated := seen[page.LastID]; repeated {
			return status, false
		}
		cursor = page.LastID
		seen[cursor] = struct{}{}
	}
	return status, false
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
		query := url.Values{"limit": {listingAnthropicLimit}}
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, listingResponseBytes+1))
	if err != nil {
		return page, HealthUnreachable
	}
	if len(body) > listingResponseBytes {
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
