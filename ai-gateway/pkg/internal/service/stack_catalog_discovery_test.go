package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
)

func TestStackCatalogLister_BestEffortInventory(t *testing.T) {
	stack := testStackCatalog()
	claude := stack.Spec.Providers["claude"]
	claude.ServeAllModels = new(true)
	claude.ModelBlocklist = []string{"blocked"}
	stack.Spec.Providers["claude"] = claude
	second := claude
	second.ModelBlocklist = nil
	stack.Spec.Providers["second"] = second
	stack.Keys["second"] = resource.RawSecureValue("second-key")
	listed := stack.Spec.Models["balanced"]
	listed.Aliases = append(listed.Aliases, "claude/org/alias")
	stack.Spec.Models["balanced"] = listed
	listed.Aliases = nil
	stack.Spec.Models["claude/override"] = listed

	type request struct{ key, path string }
	requests := make(chan request, 8)
	checker := NewCatalogHealthChecker(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		key := req.Header.Get("X-Api-Key")
		requests <- request{key, req.URL.Path}
		if key == "second-key" {
			return nil, errors.New("provider unavailable")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{
			"data":[{"id":"org/new"},{"id":"blocked"},{"id":"override"},{"id":"org/alias"},
			{"id":"org/new"},{"id":" padded"},{"id":"second-key"}],"has_more":true,"last_id":"second-key"
		}`))}, nil
	}), outbound.CustomEndpoints{})
	caller := callerContext(t, gatewayauth.CatalogAccounts)
	ctx := WithStackCatalog(caller, stack)
	lister := StackCatalogLister{Listings: checker}
	original, err := StackCatalogLister{}.ListModels(ctx)
	require.NoError(t, err)
	infos, err := lister.ListModels(ctx)
	require.NoError(t, err)
	assert.Equal(t, append(original, catalog.ModelInfo{
		ID: "claude/org/new", Name: "org/new", Candidates: []catalog.ConfiguredCandidate{
			{ProviderInstance: "claude", Provider: "anthropic", ModelID: "org/new"},
		},
	}), infos)
	require.Len(t, requests, 2)
	assert.ElementsMatch(t, []request{{"claude-catalog-key", "/v1/models"}, {"second-key", "/v1/models"}}, []request{<-requests, <-requests})

	verified, ok := gatewayauth.CallerFromContext(caller)
	require.True(t, ok)
	busy := healthCatalogID{verified.Namespace, stack.Name}
	require.True(t, checker.admit(busy))
	defer checker.release(busy)
	infos, err = lister.ListModels(ctx)
	require.NoError(t, err)
	assert.Equal(t, original, infos)
	assert.Empty(t, requests)
}

func TestStackCatalogLister_InventoryRespectsTheDiscoveryBudget(t *testing.T) {
	stack := testStackCatalog()
	claude := stack.Spec.Providers["claude"]
	claude.ServeAllModels = new(true)
	stack.Spec.Providers["claude"] = claude
	clear(stack.Spec.Models)
	for i := range config.MaxDiscoveryModelIDs - 2 {
		stack.Spec.Models[fmt.Sprintf("model-%d", i)] = v1alpha1.Model{
			Name: "Listed", Primary: v1alpha1.Target{Provider: "claude", Model: "backend"},
		}
	}
	model := stack.Spec.Models["model-0"]
	model.Aliases = []string{"alias"}
	stack.Spec.Models["model-0"] = model
	probes := make(chan struct{}, 2)
	checker := NewCatalogHealthChecker(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		probes <- struct{}{}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"z"},{"id":"a"}],"has_more":false}`))}, nil
	}), outbound.CustomEndpoints{})
	lister := StackCatalogLister{Listings: checker}
	caller := callerContext(t, gatewayauth.CatalogAccounts)
	infos, err := lister.ListModels(WithStackCatalog(caller, stack))
	require.NoError(t, err)
	require.Len(t, infos, config.MaxDiscoveryModelIDs-1)
	assert.Equal(t, "claude/a", infos[len(infos)-1].ID)
	assert.Len(t, probes, 1)

	model.Aliases = append(model.Aliases, "another-alias")
	stack.Spec.Models["model-0"] = model
	infos, err = lister.ListModels(WithStackCatalog(caller, stack))
	require.NoError(t, err)
	assert.Len(t, infos, config.MaxDiscoveryModelIDs-2)
	assert.Len(t, probes, 1, "a full configured catalog must not probe")
}
