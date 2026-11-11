package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
)

func TestManifestKinds(t *testing.T) {
	kinds, err := manifestKinds("../../../../src/app-sdk-manifest.json")
	require.NoError(t, err)
	require.Len(t, kinds, 2)
	for i, want := range []struct{ kind, plural string }{{"ModelCatalog", "ModelCatalogs"}, {"Middleware", "Middlewares"}} {
		assert.Equal(t, want.kind, kinds[i].Kind)
		assert.Equal(t, want.plural, kinds[i].Plural)
		require.NotNil(t, kinds[i].Admission)
		assert.True(t, kinds[i].Admission.SupportsAnyValidation())
		assert.Equal(t, []app.AdmissionOperation{app.AdmissionOperationCreate, app.AdmissionOperationUpdate}, kinds[i].Admission.Validation.Operations)
	}

	none, err := manifestKinds("")
	require.NoError(t, err)
	assert.Empty(t, none)
}
