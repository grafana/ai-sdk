package execution

import (
	"context"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelection_Source(t *testing.T) {
	model := &generateModel{modelID: "model"}
	resolver, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "configured"}, Model: model}})
	require.NoError(t, err)
	fromCatalog, err := CatalogSelector(resolver)(context.Background(), "configured", provider.CallOptions{}, nil)
	require.NoError(t, err)
	for _, tc := range []struct {
		name     string
		selected Selection
		want     Source
	}{
		{name: "catalog selector", selected: fromCatalog, want: SourceOperatorCatalog},
		{name: "configured selection", selected: ConfiguredSelection("configured", model, nil), want: SourceOperatorCatalog},
		{name: "account selection", selected: AccountSelection("native/model", model, nil), want: SourceRequestBYOK},
		{name: "stack catalog selection", selected: StackCatalogSelection("configured", "team", model, nil, nil), want: SourceStackCatalog},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.selected.Source)
		})
	}
}

func TestStackCatalogSelection(t *testing.T) {
	model := &generateModel{modelID: "model"}
	expected := []ExpectedAttempt{{ModelID: "alias", CanonicalID: "configured", Destination: catalog.ConfiguredCandidate{ProviderInstance: "primary", Provider: "anthropic", ModelID: "native"}}}
	keys := []string{"catalog-key"}
	selected := StackCatalogSelection("configured", "team", model, expected, keys)
	expected[0].ModelID, keys[0] = "mutated", "mutated"

	assert.Equal(t, "configured", selected.ID)
	assert.Equal(t, "team", selected.Catalog)
	assert.Equal(t, attributionConfigured, selected.attribution)
	assert.Equal(t, "alias", selected.expected[0].ModelID)
	assert.Equal(t, []string{"catalog-key"}, selected.keys)
	assert.Empty(t, ConfiguredSelection("configured", model, nil).Catalog)
	assert.Empty(t, ConfiguredSelection("configured", model, nil).keys)
	assert.Empty(t, AccountSelection("native/model", model, nil).keys)
}
