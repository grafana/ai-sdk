package providerwirev4

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	legacy "github.com/grafana/ai-sdk/gateway/providerwire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContractOnlyPackage_HasNoRuntimeAPI(t *testing.T) {
	var productionFiles []string
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		productionFiles = append(productionFiles, filepath.ToSlash(strings.TrimPrefix(path, "./")))
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					switch specification := specification.(type) {
					case *ast.TypeSpec:
						assert.False(t, specification.Name.IsExported(), "production contract type %s must not be exported", specification.Name.Name)
					case *ast.ValueSpec:
						for _, name := range specification.Names {
							assert.False(t, name.IsExported(), "production contract value %s must not be exported", name.Name)
						}
					}
				}
			case *ast.FuncDecl:
				assert.False(t, declaration.Name.IsExported(), "production contract function %s must not be exported", declaration.Name.Name)
			}
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"doc.go"}, productionFiles)
}

func TestContractOnlyPackage_StrictSyntaxIsTestOnly(t *testing.T) {
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		assert.NotContains(t, string(raw), "github.com/go-json-experiment/json")
		assert.NotContains(t, string(raw), "validateStrictJSON")
		return nil
	})
	require.NoError(t, err)
}

func TestOpenAPI_LocalContractSurface(t *testing.T) {
	raw, err := os.ReadFile("openapi.yaml")
	require.NoError(t, err)
	text := string(raw)
	assert.Equal(t, 1, strings.Count(text, "  /language-model:"))
	assert.Contains(t, text, "openapi: 3.1.0")
	assert.Contains(t, text, "application/json:")
	assert.Contains(t, text, "text/event-stream:")
	assert.Contains(t, text, "x-event-data-schema:")
	assert.Contains(t, text, "        default:\n          $ref: '#/components/responses/Error'")
	assert.Contains(t, text, "    StreamPart:\n      $ref: './schema/stream-part.json'")

	refPattern := regexp.MustCompile(`(?m)^\s*\$ref:\s*['\"]?([^'\"\s]+)`)
	for _, match := range refPattern.FindAllStringSubmatch(text, -1) {
		reference := match[1]
		if strings.HasPrefix(reference, "#/") {
			continue
		}
		require.True(t, strings.HasPrefix(reference, "./schema/"), "non-local OpenAPI reference %q", reference)
		path := strings.TrimPrefix(strings.Split(reference, "#")[0], "./")
		_, err := os.Stat(path)
		require.NoError(t, err, "missing OpenAPI reference %q", reference)
	}
}

func TestLegacyTransport_RemainsCanonicalAndAvailable(t *testing.T) {
	assert.Equal(t, "/language-model", legacy.PathLanguageModel)
	assert.Equal(t, "ai-language-model-id", legacy.HeaderModelID)
	assert.Equal(t, "ai-language-model-streaming", legacy.HeaderStreaming)
	assert.Equal(t, "ai-language-model-specification-version", legacy.HeaderSpecVersion)
	assert.Equal(t, "4", legacy.SpecVersionV4)
	assert.Equal(t, "application/json", legacy.MIMEJSON)
	assert.Equal(t, "text/event-stream", legacy.MIMESSE)
}

func TestGrafanaProvider_DefaultsToLegacyTransport(t *testing.T) {
	raw, err := os.ReadFile("../../../providers/grafana/model.go")
	require.NoError(t, err)
	text := string(raw)
	assert.Contains(t, text, `"github.com/grafana/ai-sdk/gateway/providerwire"`)
	assert.NotContains(t, text, `"github.com/grafana/ai-sdk/gateway/providerwire/v4"`)
	assert.Contains(t, text, "providerwire.EncodeCallOptions")
	assert.Contains(t, text, "providerwire.NewSSEReader")
}
