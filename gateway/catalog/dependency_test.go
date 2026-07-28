package catalog

import (
	"go/build"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageDependencies_TransportNeutral(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	require.NoError(t, err)
	assert.NotContains(t, pkg.Imports, "net/http")
	assert.NotContains(t, pkg.Imports, "github.com/grafana/ai-sdk/gateway/providerwire")
	for _, importPath := range pkg.Imports {
		assert.False(t, strings.HasPrefix(importPath, "github.com/grafana/ai-sdk/providers/"))
	}
}
