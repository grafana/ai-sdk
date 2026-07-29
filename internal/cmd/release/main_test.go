package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanFlags_ModuleSelection(t *testing.T) {
	t.Parallel()
	options, jsonOutput, err := planFlags("plan", []string{
		"--module", "providers/openai",
		"--module", "middleware/logger",
		"--prerelease", "beta",
		"--json",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"providers/openai", "middleware/logger"}, options.Modules)
	assert.Equal(t, "beta", options.Prerelease)
	assert.True(t, jsonOutput)
}
