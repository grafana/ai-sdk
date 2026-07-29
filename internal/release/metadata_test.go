package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFragment_ValidatesExplicitIntent(t *testing.T) {
	t.Parallel()
	registry := Registry{Modules: []Module{
		{ID: "core"},
		{ID: "providers/openai"},
	}}
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "valid",
			content: "---\ncore: minor\nproviders/openai: patch\n---\n\nAdd continuation support.\n",
		},
		{
			name:    "unknown module",
			content: "---\nproviders/missing: patch\n---\n\nFix it.\n",
			wantErr: "unknown module",
		},
		{
			name:    "invalid bump",
			content: "---\ncore: tiny\n---\n\nFix it.\n",
			wantErr: "invalid header",
		},
		{
			name:    "empty summary",
			content: "---\ncore: patch\n---\n",
			wantErr: "empty summary",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseFragment("change.md", []byte(tt.content), registry)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, BumpMinor, got.Bumps["core"])
			assert.Equal(t, BumpPatch, got.Bumps["providers/openai"])
			assert.Equal(t, "Add continuation support.", got.Summary)
		})
	}
}

func TestCreateFragment_SortsModulesAndDoesNotOverwrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, changesPath), 0o755))
	registry := Registry{Modules: []Module{{ID: "core"}, {ID: "providers/openai"}}}

	path, err := CreateFragment(root, "continuation", "Add continuation support.", map[string]Bump{
		"providers/openai": BumpPatch,
		"core":             BumpMinor,
	}, registry)
	require.NoError(t, err)
	assert.Equal(t, ".changes/continuation.md", path)
	content, err := os.ReadFile(filepath.Join(root, path))
	require.NoError(t, err)
	assert.Equal(t, "---\ncore: minor\nproviders/openai: patch\n---\n\nAdd continuation support.\n", string(content))

	_, err = CreateFragment(root, "continuation", "Another entry.", map[string]Bump{"core": BumpPatch}, registry)
	require.Error(t, err)
}

func TestLocalReplace_DetectsSingleAndBlockForms(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "single local", content: "replace github.com/grafana/ai-sdk => ../../\n", want: true},
		{name: "block local", content: "replace (\n github.com/grafana/ai-sdk => ../../\n)\n", want: true},
		{name: "versioned remote", content: "replace github.com/grafana/ai-sdk v0.1.0 => github.com/acme/fork v0.1.1\n"},
		{name: "unrelated local", content: "replace example.com/a => ../a\n", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, localReplace([]byte(tt.content)))
		})
	}
}

func TestValidateRegistry_RejectsUnregisteredPublicModule(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	registry := testRegistry()
	writeTestFile(t, root, "go.mod", "module github.com/grafana/ai-sdk\n")
	writeTestFile(t, root, "CHANGELOG.md", "# Changelog\n")
	writeTestFile(t, root, "providers/openai/go.mod", "module github.com/grafana/ai-sdk/providers/openai\n")
	writeTestFile(t, root, "providers/openai/CHANGELOG.md", "# Changelog\n")
	writeTestFile(t, root, "providers/unregistered/go.mod", "module github.com/grafana/ai-sdk/providers/unregistered\n")
	writeTestFile(t, root, "middleware/.gitkeep", "")

	err := ValidateRegistry(root, registry)
	require.ErrorContains(t, err, "unregistered public modules: providers/unregistered")
}
