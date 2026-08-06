package providerwirev4

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictPackage_DoesNotImportRemovedGatewayLayers(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.ImportsOnly)
		require.NoError(t, err)
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			assert.NotContains(t, []string{
				"github.com/grafana/ai-sdk/gateway/providerwire",
				"github.com/grafana/ai-sdk/gateway/runtime",
				"github.com/grafana/ai-sdk/gateway/failure",
			}, path)
		}
	}
}
