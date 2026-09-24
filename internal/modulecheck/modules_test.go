package modulecheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRepo(t *testing.T, roots ...string) string {
	t.Helper()
	repo := t.TempDir()
	for _, root := range roots {
		path := modulePrefix
		if root != "." {
			path += "/" + root
		}
		file := filepath.Join(repo, root, "go.mod")
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
		require.NoError(t, os.WriteFile(file, []byte("module "+path+"\n\ngo 1.26.3\n"), 0o644))
	}
	for _, args := range [][]string{{"init"}, {"add", "."}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
	}
	return repo
}

func TestDiscover_ClassifiesTrackedModules(t *testing.T) {
	repo := testRepo(t, ".", "ai-gateway", "providers/openai", "middleware/logger", "examples/demo", "test/conformance", "ai-gateway/extra")
	modules, err := Discover(repo)
	require.NoError(t, err)
	require.Len(t, modules, 7)
	for _, tc := range []struct {
		selection string
		published bool
	}{
		{modulePrefix, true}, {"ai-gateway", true}, {"providers/openai", true},
		{"middleware/logger", true}, {"examples/demo", false}, {"test/conformance", false},
		{"unknown", false},
	} {
		t.Run(tc.selection, func(t *testing.T) {
			selection, err := Select(modules, tc.selection)
			if tc.published {
				require.NoError(t, err)
				require.Len(t, selection, 1)
			} else {
				require.Error(t, err)
			}
		})
	}
	all, err := Select(modules, "")
	require.NoError(t, err)
	assert.Len(t, all, 5)
	owner, ok := Owner(modules, "ai-gateway/extra/file.go")
	require.True(t, ok)
	assert.Equal(t, "ai-gateway/extra", owner.Root)
	owner, ok = Owner(modules, "provider/language_model.go")
	require.True(t, ok)
	assert.Equal(t, ".", owner.Root)
}

func TestCheckSourceImports_NearestModuleBoundary(t *testing.T) {
	repo := testRepo(t, ".", "ai-gateway", "ai-gateway/extra", "providers/grafana")
	gatewayFile := filepath.Join(repo, "ai-gateway/gateway.go")
	require.NoError(t, os.WriteFile(gatewayFile, []byte("package gateway\nimport _ \""+modulePrefix+"/ai-gateway/catalog\"\n"), 0o644))
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	modules, err := Discover(repo)
	require.NoError(t, err)
	require.NoError(t, CheckSourceImports(repo, modules))
	for _, path := range []string{"providers/grafana/reverse.go", "ai-gateway/extra/reverse.go"} {
		t.Run(path, func(t *testing.T) {
			file := filepath.Join(repo, path)
			require.NoError(t, os.WriteFile(file, []byte("package check\nimport _ \""+modulePrefix+"/ai-gateway/catalog\"\n"), 0o644))
			cmd := exec.Command("git", "add", path)
			cmd.Dir = repo
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
			err = CheckSourceImports(repo, modules)
			require.ErrorContains(t, err, "imports the AI Gateway module")
			require.NoError(t, os.Remove(file))
			cmd = exec.Command("git", "rm", "--cached", path)
			cmd.Dir = repo
			output, err = cmd.CombinedOutput()
			require.NoError(t, err, string(output))
		})
	}
}

func TestCheckModuleReferences_NestedModule(t *testing.T) {
	repo := testRepo(t, ".", "ai-gateway", "ai-gateway/extra")
	modules, err := Discover(repo)
	require.NoError(t, err)
	require.NoError(t, CheckModuleReferences(repo, modules))
	file := filepath.Join(repo, "ai-gateway/extra/go.mod")
	for _, tc := range []struct {
		name string
		text string
	}{
		{"require", "require " + modulePrefix + "/ai-gateway v0.0.0\n"},
		{"module replacement", "replace " + modulePrefix + "/ai-gateway => example.com/other v1.0.0\n"},
		{"source replacement", "replace example.com/other => ../\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(file, []byte("module "+modulePrefix+"/ai-gateway/extra\n\ngo 1.26.3\n"+tc.text), 0o644))
			require.ErrorContains(t, CheckModuleReferences(repo, modules), "AI Gateway")
		})
	}
}

func TestDiscover_RejectsMisdeclaredModule(t *testing.T) {
	repo := testRepo(t, ".", "providers/openai")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "providers/openai/go.mod"), []byte("module github.com/example/other\n"), 0o644))
	_, err := Discover(repo)
	require.ErrorContains(t, err, "invalid or duplicate module path")
}
