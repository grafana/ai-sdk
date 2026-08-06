package runtime

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

func TestTransportNeutralDependencyBoundaries(t *testing.T) {
	cases := []struct {
		name      string
		directory string
		forbidden []string
	}{
		{
			name:      "failure",
			directory: "../failure",
			forbidden: []string{"net/http", "github.com/grafana/ai-sdk/gateway/providerwire"},
		},
		{
			name:      "runtime",
			directory: ".",
			forbidden: []string{
				"net/http",
				"github.com/grafana/ai-sdk/gateway/providerwire",
				"github.com/grafana/ai-sdk/providers",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matches, err := filepath.Glob(filepath.Join(tc.directory, "*.go"))
			require.NoError(t, err)
			for _, filename := range matches {
				if strings.HasSuffix(filename, "_test.go") {
					continue
				}
				file, err := parser.ParseFile(token.NewFileSet(), filename, nil, parser.ImportsOnly)
				require.NoError(t, err)
				for _, spec := range file.Imports {
					path, err := strconv.Unquote(spec.Path.Value)
					require.NoError(t, err)
					for _, forbidden := range tc.forbidden {
						assert.Falsef(t, path == forbidden || strings.HasPrefix(path, forbidden+"/"), "%s imports forbidden dependency %s", filename, path)
					}
				}
			}
		})
	}
}
