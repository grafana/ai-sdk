package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

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
	lister := NewStackCatalogLister(roundTripFunc(func(req *http.Request) (*http.Response, error) {
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
	require.True(t, lister.listings.checker.admit(busy))
	defer lister.listings.checker.release(busy)
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
	lister := NewStackCatalogLister(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		probes <- struct{}{}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"z"},{"id":"a"}],"has_more":false}`))}, nil
	}), outbound.CustomEndpoints{})
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

func discoveryStack() StackCatalog {
	stack := testStackCatalog()
	provider := stack.Spec.Providers["claude"]
	provider.ServeAllModels = new(true)
	stack.Spec.Providers = map[string]v1alpha1.Provider{"claude": provider}
	stack.Spec.Models = map[string]v1alpha1.Model{"same": stack.Spec.Models["same"]}
	delete(stack.Keys, "gpt")
	stack.ResourceVersion = "7"
	return stack
}

func discoveryListingResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

func TestStackCatalogLister_IsolatedAdmission(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command func(*CatalogHealthChecker, context.Context, string, StackCatalog) error
	}{
		{name: "health", command: func(c *CatalogHealthChecker, ctx context.Context, namespace string, stack StackCatalog) error {
			_, err := c.Check(ctx, namespace, stack)
			return err
		}},
		{name: "picker", command: func(c *CatalogHealthChecker, ctx context.Context, namespace string, stack StackCatalog) error {
			_, err := c.AvailableModels(ctx, namespace, stack, "claude")
			return err
		}},
	} {
		for _, discoveryFirst := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/discovery-first-%t", tc.name, discoveryFirst), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					started, release := make(chan struct{}), make(chan struct{})
					var calls atomic.Int32
					transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
						if calls.Add(1) == 1 {
							close(started)
							select {
							case <-release:
							case <-r.Context().Done():
								return nil, r.Context().Err()
							}
						}
						return discoveryListingResponse(`{"data":[{"id":"new"}],"has_more":false}`), nil
					})
					lister := NewStackCatalogLister(transport, outbound.CustomEndpoints{})
					checker := NewCatalogHealthChecker(transport, outbound.CustomEndpoints{})
					stack := discoveryStack()
					ctx := WithStackCatalog(callerContext(t, gatewayauth.CatalogAccounts), stack)
					caller, ok := gatewayauth.CallerFromContext(ctx)
					require.True(t, ok)
					discover := func() error {
						infos, err := lister.ListModels(ctx)
						assert.Len(t, infos, 2)
						return err
					}
					admin := func() error { return tc.command(checker, ctx, caller.Namespace, stack) }
					first, second := discover, admin
					if !discoveryFirst {
						first, second = admin, discover
					}
					done := make(chan error, 1)
					go func() { done <- first() }()
					<-started
					assert.NoError(t, second())
					close(release)
					assert.NoError(t, <-done)
					assert.EqualValues(t, 2, calls.Load())
				})
			})
		}
	}
}

func TestStackCatalogLister_ProcessAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		var calls atomic.Int32
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if calls.Add(1) <= healthMaxRuns {
				select {
				case <-release:
				case <-req.Context().Done():
					return nil, req.Context().Err()
				}
			}
			return discoveryListingResponse(`{"data":[{"id":"new"}],"has_more":false}`), nil
		})
		lister := NewStackCatalogLister(transport, outbound.CustomEndpoints{})
		checker := NewCatalogHealthChecker(transport, outbound.CustomEndpoints{})
		ctx := callerContext(t, gatewayauth.CatalogAccounts)
		caller, ok := gatewayauth.CallerFromContext(ctx)
		require.True(t, ok)
		stack := discoveryStack()
		done := make(chan error, healthMaxRuns)
		for i := range healthMaxRuns {
			active := stack
			active.Name = fmt.Sprintf("catalog-%d", i)
			go func() {
				_, err := lister.ListModels(WithStackCatalog(ctx, active))
				done <- err
			}()
		}
		synctest.Wait()
		assert.EqualValues(t, healthMaxRuns, calls.Load())
		for _, name := range []string{"catalog-0", "another-catalog"} {
			stack.Name = name
			infos, err := lister.ListModels(WithStackCatalog(ctx, stack))
			assert.NoError(t, err)
			assert.Len(t, infos, 1)
		}
		assert.EqualValues(t, healthMaxRuns, calls.Load())
		stack.Name = "catalog-0"
		_, err := checker.AvailableModels(ctx, caller.Namespace, stack, "claude")
		assert.NoError(t, err)
		assert.EqualValues(t, healthMaxRuns+1, calls.Load())
		close(release)
		for range healthMaxRuns {
			assert.NoError(t, <-done)
		}
	})
}

func TestStackCatalogLister_WarmInventorySkipsAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var probes atomic.Int32
		lister := NewStackCatalogLister(roundTripFunc(func(*http.Request) (*http.Response, error) {
			probes.Add(1)
			return discoveryListingResponse(`{"data":[{"id":"new"}],"has_more":false}`), nil
		}), outbound.CustomEndpoints{})
		stack := discoveryStack()
		ctx := WithStackCatalog(callerContext(t, gatewayauth.CatalogAccounts), stack)
		warm, err := lister.ListModels(ctx)
		require.NoError(t, err)
		require.Len(t, warm, 2)
		caller, ok := gatewayauth.CallerFromContext(ctx)
		require.True(t, ok)
		id := healthCatalogID{caller.Namespace, stack.Name}
		require.True(t, lister.listings.checker.admit(id))
		defer lister.listings.checker.release(id)
		for range 2 {
			go func() {
				infos, err := lister.ListModels(ctx)
				assert.NoError(t, err)
				assert.Equal(t, warm, infos)
			}()
		}
		synctest.Wait()
		assert.EqualValues(t, 1, probes.Load())
	})
}

func TestStackCatalogLister_CachedRowsSurviveBusyMiss(t *testing.T) {
	stack := discoveryStack()
	stack.Spec.Providers["cold"] = stack.Spec.Providers["claude"]
	stack.Keys["cold"] = resource.RawSecureValue("dummy-cold-key")
	ctx := WithStackCatalog(callerContext(t, gatewayauth.CatalogAccounts), stack)
	caller, ok := gatewayauth.CallerFromContext(ctx)
	require.True(t, ok)
	id := healthCatalogID{caller.Namespace, stack.Name}
	var probes atomic.Int32
	lister := NewStackCatalogLister(roundTripFunc(func(*http.Request) (*http.Response, error) {
		probes.Add(1)
		return nil, errors.New("unexpected probe")
	}), outbound.CustomEndpoints{})
	lister.listings.remember(discoveryInventoryKey{catalog: id, revision: stack.ResourceVersion, provider: "claude"}, []string{"warm"})
	require.True(t, lister.listings.checker.admit(id))
	defer lister.listings.checker.release(id)
	infos, err := lister.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, infos, 2)
	assert.Equal(t, "claude/warm", infos[1].ID)
	assert.Zero(t, probes.Load())
}

func TestStackCatalogLister_InventoryCache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			call := calls.Add(1)
			if call == 1 {
				return nil, errors.New("provider unavailable")
			}
			return discoveryListingResponse(fmt.Sprintf(`{"data":[{"id":"model-%d"}],"has_more":false}`, call)), nil
		})
		lister := NewStackCatalogLister(transport, outbound.CustomEndpoints{})
		stack := discoveryStack()
		ctx := callerContext(t, gatewayauth.CatalogAccounts)
		check := func(want int) {
			t.Helper()
			infos, err := lister.ListModels(WithStackCatalog(ctx, stack))
			require.NoError(t, err)
			if want == 0 {
				assert.Len(t, infos, 1)
				return
			}
			require.Len(t, infos, 2)
			assert.Equal(t, fmt.Sprintf("claude/model-%d", want), infos[1].ID)
		}
		check(0)
		check(2)
		time.Sleep(discoveryInventoryTTL - time.Second)
		check(2)
		assert.EqualValues(t, 2, calls.Load())
		time.Sleep(time.Second)
		check(3)
		stack.ResourceVersion = "8"
		check(4)
		stack.ResourceVersion = ""
		check(5)
		check(6)
		assert.EqualValues(t, 6, calls.Load())
	})
}

func TestStackCatalogLister_PartialInventoryCache(t *testing.T) {
	for _, tc := range []struct {
		name, page string
		wantCalls  int32
	}{
		{name: "bounded partial", page: `{"data":[{"id":"new"}],"has_more":true}`, wantCalls: 1},
		{name: "failed continuation", page: `{"data":[{"id":"new"}],"has_more":true,"last_id":"new"}`, wantCalls: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			lister := NewStackCatalogLister(roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				if req.URL.Query().Get("after_id") != "" {
					return nil, errors.New("continuation failed")
				}
				return discoveryListingResponse(tc.page), nil
			}), outbound.CustomEndpoints{})
			ctx := WithStackCatalog(callerContext(t, gatewayauth.CatalogAccounts), discoveryStack())
			for range 2 {
				infos, err := lister.ListModels(ctx)
				require.NoError(t, err)
				require.Len(t, infos, 2)
				assert.Equal(t, "claude/new", infos[1].ID)
			}
			assert.Equal(t, tc.wantCalls, calls.Load())
		})
	}
}

func TestStackCatalogLister_InventoryEvictsTheOldestEntry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		lister := NewStackCatalogLister(nil, outbound.CustomEndpoints{})
		cache := lister.listings
		key := func(i int) discoveryInventoryKey { return discoveryInventoryKey{revision: fmt.Sprint(i)} }
		for i := range discoveryMaxInventories {
			cache.remember(key(i), []string{"id"})
			time.Sleep(time.Millisecond)
		}
		cache.remember(key(discoveryMaxInventories), []string{"new"})
		assert.Len(t, cache.entries, discoveryMaxInventories)
		_, ok := cache.cached(key(0))
		assert.False(t, ok)
		ids, ok := cache.cached(key(discoveryMaxInventories))
		assert.True(t, ok)
		assert.Equal(t, []string{"new"}, ids)
	})
}
