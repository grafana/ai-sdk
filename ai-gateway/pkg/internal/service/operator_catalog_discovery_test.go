package service

import (
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
)

func TestOperatorCatalogLister_WarmInventorySkipsAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		file := config.File{Providers: map[string]config.Provider{"claude": {Type: "anthropic", ServeAllModels: true}}}
		accounts := map[string]config.ResolvedProvider{"claude": {Type: "anthropic", APIKey: "dummy-key"}}
		models, err := BuildCatalog(file, accounts, nil, ConfiguredIdentity)
		require.NoError(t, err)
		var probes atomic.Int32
		transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
			probes.Add(1)
			return discoveryListingResponse(`{"data":[{"id":"new"}],"has_more":false}`), nil
		})
		var lister operatorCatalogLister
		switch built := models.DiscoveryLister(transport).(type) {
		case operatorCatalogLister:
			lister = built
		default:
			require.FailNow(t, "opted-in catalog must use inventory discovery")
		}
		warm, err := lister.ListModels(t.Context())
		require.NoError(t, err)
		require.Len(t, warm, 1)
		assert.Equal(t, "claude/new", warm[0].ID)
		id := healthCatalogID{}
		require.True(t, lister.listings.checker.admit(id))
		defer lister.listings.checker.release(id)
		for range 2 {
			go func() {
				infos, err := lister.ListModels(t.Context())
				assert.NoError(t, err)
				assert.Equal(t, warm, infos)
			}()
		}
		synctest.Wait()
		assert.EqualValues(t, 1, probes.Load())
	})
}
