package grafana

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicSurface_ModuleIsolation(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		require.NoError(t, err)
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			require.NoError(t, err)
			assert.NotContains(t, path, "/ai-"+"gateway")
			assert.NotContains(t, path, "/gateway/"+"providerwire")
		}
		ast.Inspect(file, func(node ast.Node) bool {
			typeSpec, ok := node.(*ast.TypeSpec)
			if !ok || !typeSpec.Name.IsExported() {
				return true
			}
			for _, forbidden := range []string{"Wire", "Codec", "DTO", "StreamReader", "Transport"} {
				assert.NotContains(t, typeSpec.Name.Name, forbidden)
			}
			assert.False(t, strings.HasSuffix(typeSpec.Name.Name, "Mode"))
			return true
		})
	}
	mod, err := os.ReadFile(filepath.Join("go.mod"))
	require.NoError(t, err)
	assert.NotContains(t, string(mod), "github.com/grafana/ai-sdk/"+"ai-gateway")
	assert.Contains(t, string(mod), "module github.com/grafana/ai-sdk/providers/grafana")
}

func TestPublicSurface_NoClientLimits(t *testing.T) {
	configs := map[string]bool{"TokenExchangeConfig": false, "ServiceAccountConfig": false, "AccessTokenConfig": false}
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		require.NoError(t, err)
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.TypeSpec:
				assert.NotEqual(t, "Limits", n.Name.Name)
				structType, ok := n.Type.(*ast.StructType)
				if _, config := configs[n.Name.Name]; ok && config {
					configs[n.Name.Name] = true
					for _, field := range structType.Fields.List {
						for _, name := range field.Names {
							assert.NotContains(t, name.Name, "Limit", n.Name.Name)
						}
					}
				}
			case *ast.FuncDecl:
				assert.NotEqual(t, "DefaultLimits", n.Name.Name)
			}
			return true
		})
	}
	for name, found := range configs {
		assert.True(t, found, name)
	}
}
