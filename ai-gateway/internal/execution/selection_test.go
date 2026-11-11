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
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.selected.Source)
		})
	}
}
