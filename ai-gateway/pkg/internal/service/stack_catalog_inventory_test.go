package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
)

func TestAvailableModels_ListsAllPagesRatherThanOnlyHealthTargets(t *testing.T) {
	calls := 0
	checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		assert.Equal(t, "/v1/models", r.URL.Path)
		assert.Equal(t, "claude-catalog-key", r.Header.Get("X-Api-Key"))
		if r.URL.Query().Get("after_id") == "" {
			return listingResponse(200, `{"data":[{"id":"claude-sonnet"},{"id":"Z"}],"has_more":true,"last_id":"cursor+literal@v2&more"}`), nil
		}
		assert.Equal(t, "cursor+literal@v2&more", r.URL.Query().Get("after_id"))
		return listingResponse(200, `{"data":[{"id":"Z"},{"id":"a/B"}],"has_more":false}`), nil
	}), outbound.CustomEndpoints{})
	for range 2 {
		result, err := checker.AvailableModels(t.Context(), "stack", testStackCatalog(), "claude")
		require.NoError(t, err)
		assert.Equal(t, CatalogProviderModels{Status: ListingOK, ModelIDs: []string{"Z", "a/B", "claude-sonnet"}}, result)
	}
	assert.Equal(t, 4, calls, "listing is not cached")
}

func TestAvailableModels_CompletionAndSecretHandling(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
		want CatalogProviderModels
	}{
		{"empty", 200, `{"data":[],"has_more":false}`, CatalogProviderModels{ListingOK, []string{}}},
		{"invalid key", 401, "claude-catalog-key", CatalogProviderModels{ListingInvalidKey, []string{}}},
		{"refused", 403, "private response", CatalogProviderModels{ListingRefused, []string{}}},
		{"unreachable", 503, "private response", CatalogProviderModels{ListingUnreachable, []string{}}},
		{"malformed", 200, `{"data":[{}],"has_more":false}`, CatalogProviderModels{ListingRefused, []string{}}},
		{"oversized body", 200, strings.Repeat(" ", listingResponseBytes+1), CatalogProviderModels{ListingNotChecked, []string{}}},
		{"missing cursor", 200, `{"data":[],"has_more":true}`, CatalogProviderModels{ListingPartial, []string{}}},
		{"unsafe cursor", 200, `{"data":[{"id":"safe"}],"has_more":true,"last_id":"prefix-claude-catalog-key"}`, CatalogProviderModels{ListingPartial, []string{"safe"}}},
		{"oversized cursor", 200, `{"data":[],"has_more":true,"last_id":"` + strings.Repeat("a", listingCursorBytes+1) + `"}`, CatalogProviderModels{ListingPartial, []string{}}},
		{"unsafe IDs", 200, `{"data":[{"id":"safe"},{"id":"claude-catalog-key"},{"id":"embed-gpt-catalog-key"},{"id":"a\u0000b"},{"id":"` + strings.Repeat("a", config.MaxListedModelIDBytes+1) + `"}],"has_more":false}`, CatalogProviderModels{ListingPartial, []string{"safe"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, 1, calls)
				assert.Empty(t, r.URL.Query().Get("after_id"))
				return listingResponse(tc.code, tc.body), nil
			}), outbound.CustomEndpoints{})
			result, err := checker.AvailableModels(t.Context(), "stack", testStackCatalog(), "claude")
			require.NoError(t, err)
			assert.Equal(t, tc.want, result)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestAvailableModels_ProtectsSavedKeysInIDsAndCursors(t *testing.T) {
	for _, cursor := range []string{"selected-key", "other-key", "other-key \t"} {
		t.Run(fmt.Sprintf("cursor %q", cursor), func(t *testing.T) {
			stack := testStackCatalog()
			stack.Keys["claude"] = " \tselected-key\t "
			stack.Keys["gpt"] = "other-key \t"
			encodedCursor, err := json.Marshal(cursor)
			require.NoError(t, err)
			var calls int
			checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, 1, calls, "continuation must stop")
				assert.Empty(t, r.URL.Query().Get("after_id"))
				body := fmt.Sprintf(`{"data":[{"id":"id-selected-key"},{"id":"id-other-key"},{"id":"listed"}],"has_more":true,"last_id":%s}`, encodedCursor)
				return listingResponse(200, body), nil
			}), outbound.CustomEndpoints{})
			result, err := checker.AvailableModels(t.Context(), "stack", stack, "claude")
			require.NoError(t, err)
			assert.Equal(t, CatalogProviderModels{ListingPartial, []string{"listed"}}, result)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestAvailableModels_CapsInventory(t *testing.T) {
	ids := make([]map[string]string, 0, maxListedModelIDs+1)
	for i := range maxListedModelIDs + 1 {
		ids = append(ids, map[string]string{"id": fmt.Sprintf("model-%04d", i)})
	}
	body, err := json.Marshal(map[string]any{"data": ids})
	require.NoError(t, err)
	checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "Bearer gpt-catalog-key", r.Header.Get("Authorization"))
		return listingResponse(200, string(body)), nil
	}), outbound.CustomEndpoints{})
	result, err := checker.AvailableModels(t.Context(), "stack", testStackCatalog(), "gpt")
	require.NoError(t, err)
	assert.Equal(t, ListingPartial, result.Status)
	require.Len(t, result.ModelIDs, maxListedModelIDs)
	assert.Equal(t, "model-0000", result.ModelIDs[0])
	assert.Equal(t, "model-0511", result.ModelIDs[maxListedModelIDs-1])
}

func TestCatalogHealthChecker_AvailableModelsHonorsTheEndpointList(t *testing.T) {
	stack := testStackCatalog()
	stack.Spec.Providers["claude"] = v1alpha1.Provider{Type: "anthropic", BaseURL: new("https://anthropic-proxy.example.com/team")}
	t.Run("a listed endpoint is listed", func(t *testing.T) {
		endpoints, err := outbound.ParseCustomEndpoints("https://anthropic-proxy.example.com/team")
		require.NoError(t, err)
		var urls []string
		checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
			urls = append(urls, r.URL.Scheme+"://"+r.URL.Host+r.URL.Path)
			return listingResponse(200, `{"data":[{"id":"claude-sonnet"}],"has_more":false}`), nil
		}), endpoints)
		result, err := checker.AvailableModels(t.Context(), "stack", stack, "claude")
		require.NoError(t, err)
		assert.Equal(t, ListingOK, result.Status)
		assert.Equal(t, []string{"claude-sonnet"}, result.ModelIDs)
		assert.Equal(t, []string{"https://anthropic-proxy.example.com/team/v1/models"}, urls)
	})
	t.Run("an unlisted endpoint is refused before any request", func(t *testing.T) {
		checker := NewCatalogHealthChecker(selectionTransport(func(*http.Request) (*http.Response, error) {
			t.Error("an unlisted endpoint attempted a provider request")
			return nil, errors.New("unexpected provider request")
		}), outbound.CustomEndpoints{})
		for _, provider := range []string{"claude", "gpt"} {
			_, err := checker.AvailableModels(t.Context(), "stack", stack, provider)
			require.ErrorIs(t, err, execution.ErrCatalogConfiguration, provider)
		}
	})
}
