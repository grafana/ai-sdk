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
	require.Len(t, kinds, 1)
	assert.Equal(t, "ModelCatalog", kinds[0].Kind)
	assert.Equal(t, "ModelCatalogs", kinds[0].Plural)
	require.NotNil(t, kinds[0].Admission)
	assert.True(t, kinds[0].Admission.SupportsAnyValidation())
	assert.Equal(t, []app.AdmissionOperation{app.AdmissionOperationCreate, app.AdmissionOperationUpdate}, kinds[0].Admission.Validation.Operations)

	none, err := manifestKinds("")
	require.NoError(t, err)
	assert.Empty(t, none)
}
