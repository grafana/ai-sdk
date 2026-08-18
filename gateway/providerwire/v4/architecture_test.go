package providerwirev4

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
