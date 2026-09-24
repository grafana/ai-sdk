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
}

func TestDiscover_RejectsMisdeclaredModule(t *testing.T) {
	repo := testRepo(t, ".", "providers/openai")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "providers/openai/go.mod"), []byte("module github.com/example/other\n"), 0o644))
	_, err := Discover(repo)
	require.ErrorContains(t, err, "invalid or duplicate module path")
}
