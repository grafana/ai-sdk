package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAvailableModels_ListsAllPagesRatherThanOnlyHealthTargets(t *testing.T) {
	calls := 0
	checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		assert.Equal(t, "/v1/models", r.URL.Path)
		assert.Equal(t, "claude-catalog-key", r.Header.Get("X-Api-Key"))
		if r.URL.Query().Get("after_id") == "" {
			return listingResponse(200, `{"data":[{"id":"claude-sonnet"},{"id":"Z"}],"has_more":true,"last_id":"cursor&literal"}`), nil
		}
		assert.Equal(t, "cursor&literal", r.URL.Query().Get("after_id"))
		return listingResponse(200, `{"data":[{"id":"Z"},{"id":"a/B"}],"has_more":false}`), nil
	}))
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
		{"unsafe IDs", 200, `{"data":[{"id":"safe"},{"id":"claude-catalog-key"},{"id":"embed-gpt-catalog-key"},{"id":"a\u0000b"},{"id":"` + strings.Repeat("a", maxListedModelBytes+1) + `"}],"has_more":false}`, CatalogProviderModels{ListingPartial, []string{"safe"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, 1, calls)
				assert.Empty(t, r.URL.Query().Get("after_id"))
				return listingResponse(tc.code, tc.body), nil
			}))
			result, err := checker.AvailableModels(t.Context(), "stack", testStackCatalog(), "claude")
			require.NoError(t, err)
			assert.Equal(t, tc.want, result)
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
	}))
	result, err := checker.AvailableModels(t.Context(), "stack", testStackCatalog(), "gpt")
	require.NoError(t, err)
	assert.Equal(t, ListingPartial, result.Status)
	require.Len(t, result.ModelIDs, maxListedModelIDs)
	assert.Equal(t, "model-0000", result.ModelIDs[0])
	assert.Equal(t, "model-0511", result.ModelIDs[maxListedModelIDs-1])
}
