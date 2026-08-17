package providerwirev4

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictPackage_ProductionDependencies(t *testing.T) {
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
			if strings.Contains(path, ".") {
				assert.Equal(t, "github.com/grafana/ai-sdk/provider", path)
			}
		}
	}
}

func TestStrictPackage_PublicSurface(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	var exported []string
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
		require.NoError(t, err)
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if declaration.Recv == nil && declaration.Name.IsExported() {
					exported = append(exported, declaration.Name.Name)
				}
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					switch specification := specification.(type) {
					case *ast.TypeSpec:
						if specification.Name.IsExported() {
							exported = append(exported, specification.Name.Name)
						}
					case *ast.ValueSpec:
						for _, name := range specification.Names {
							if name.IsExported() {
								exported = append(exported, name.Name)
							}
						}
					}
				}
			}
		}
	}
	sort.Strings(exported)
	assert.Equal(t, []string{"EncodeCallOptions"}, exported)
}
