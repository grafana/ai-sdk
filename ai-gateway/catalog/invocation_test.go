package catalog

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticCatalog_InvocationContext(t *testing.T) {
	candidates := []ConfiguredCandidate{{Provider: "anthropic", ProviderInstance: "primary", ModelID: "backend"}}
	sources := []string{"actual-credential"}
	entries := []StaticEntry{{Info: ModelInfo{ID: "public", Aliases: []string{"alias"}, Candidates: candidates}, Model: &catalogTestModel{providerName: "anthropic", modelID: "backend"}, ProtectedSources: sources}}
	created, err := NewStatic(entries)
	require.NoError(t, err)
	candidates[0].ModelID = "mutated"
	sources[0] = "mutated"
	resolved, err := created.ResolveModel(context.Background(), "alias")
	require.NoError(t, err)
	assert.Equal(t, []ConfiguredCandidate{{Provider: "anthropic", ProviderInstance: "primary", ModelID: "backend"}}, resolved.Candidates)
	assert.Equal(t, []string{"actual-credential"}, resolved.ProtectedSources)
	encoded, err := json.Marshal(resolved)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "actual-credential")
	listed, err := created.ListModels(context.Background())
	require.NoError(t, err)
	encoded, err = json.Marshal(listed)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "actual-credential")
	resolved.Candidates[0].ModelID = "local"
	resolved.ProtectedSources[0] = "local"
	again, err := created.ResolveModel(context.Background(), "public")
	require.NoError(t, err)
	assert.Equal(t, "backend", again.Candidates[0].ModelID)
	assert.Equal(t, "actual-credential", again.ProtectedSources[0])
}
