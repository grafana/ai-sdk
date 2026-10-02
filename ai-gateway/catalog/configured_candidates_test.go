package catalog

import (
	"context"
	"sync"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type catalogRequestKey struct{}

func TestCatalog_ConfiguredCandidateCopies(t *testing.T) {
	for _, kind := range []string{"static", "registry"} {
		t.Run(kind, func(t *testing.T) {
			info := ModelInfo{ID: "public", Aliases: []string{"alias"}, Candidates: []ConfiguredCandidate{{ProviderInstance: "primary", Provider: "anthropic", ModelID: "configured-not-reported"}, {ProviderInstance: "backup", Provider: "openai", ModelID: "backup"}}}
			expected := append([]ConfiguredCandidate(nil), info.Candidates...)
			model := &catalogTestModel{modelID: "response-not-configured"}
			var created Catalog
			var err error
			if kind == "static" {
				created, err = NewStatic([]StaticEntry{{Info: info, Model: model}})
			} else {
				created, err = NewRegistry(&catalogTestProvider{resolve: func(string) (provider.LanguageModel, error) { return model, nil }}, []RegistryRoute{{Info: info, ProviderModelID: "registry-destination"}})
			}
			require.NoError(t, err)
			info.Candidates[0].ModelID = "mutated"
			info.Aliases[0] = "mutated"
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), catalogRequestKey{}, "request-secret-marker"))
			rows, err := created.ListModels(ctx)
			cancel()
			require.NoError(t, err)
			assert.Equal(t, expected, rows[0].Candidates)
			rows[0].Candidates[0].ProviderInstance = "mutated"
			rows[0].Aliases[0] = "local alias mutation"
			resolved, err := created.ResolveModel(context.Background(), "alias")
			require.NoError(t, err)
			assert.Equal(t, "public", resolved.ID)
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					for range 10 {
						listed, err := created.ListModels(context.Background())
						assert.NoError(t, err)
						assert.Equal(t, expected, listed[0].Candidates)
						assert.Equal(t, []string{"alias"}, listed[0].Aliases)
						listed[0].Candidates[0].ModelID = "local mutation"
					}
				})
			}
			wg.Wait()
		})
	}
}
