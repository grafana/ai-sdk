package candidate

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func requireTag(t *testing.T, repo, tag string) {
	t.Helper()
	ref := "refs/tags/" + tag
	if exec.Command("git", "-C", repo, "rev-parse", "--verify", "--quiet", ref+"^{commit}").Run() == nil {
		return
	}
	fetch := exec.Command("git", "-C", repo, "fetch", "--quiet", "--no-tags", "--depth=1", "origin", ref+":"+ref)
	if out, err := fetch.CombinedOutput(); err != nil {
		t.Skipf("published tag %s is unavailable: %v: %s", tag, err, out)
	}
}

func TestProxyAdd_MatchesPublishedTags(t *testing.T) {
	repo := repositoryRoot(t)
	tests := []struct {
		name    string
		module  Module
		tag     string
		version string
		want    Checksums
	}{
		{
			name:    "root",
			module:  Module{Dir: ".", Path: "github.com/grafana/ai-sdk"},
			tag:     "v0.1.0-alpha.3",
			version: "v0.1.0-alpha.3",
			want: Checksums{
				Zip:   "h1:eQ3U/xxETG3SICyW19KUdBrDhEOInJ0Wxyz/bnjopuo=",
				GoMod: "h1:2D9ae2gO9kFiNY21V+4g3/+dQtVIkDXvY7dlKr8Cn1Y=",
			},
		},
		{
			name:    "nested module without its own license",
			module:  Module{Dir: "providers/openai", Path: "github.com/grafana/ai-sdk/providers/openai"},
			tag:     "providers/openai/v0.1.0-alpha.1",
			version: "v0.1.0-alpha.1",
			want: Checksums{
				Zip:   "h1:PGuWbTYnMjRt5D1FgOAMaiSo2uTpFLozt6bUoyCFu6g=",
				GoMod: "h1:zRBaQVKfQmznIwal8mORjaEjz4ZaUoECS+gRxdeXU9E=",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requireTag(t, repo, tc.tag)
			got, err := Proxy{Dir: t.TempDir()}.Add(repo, "refs/tags/"+tc.tag, tc.module, tc.version)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestProxyAdd_Layout(t *testing.T) {
	repo := repositoryRoot(t)
	proxy := Proxy{Dir: t.TempDir()}
	m := Module{Dir: "middleware/logger", Path: "github.com/grafana/ai-sdk/middleware/logger"}

	_, err := proxy.Add(repo, "HEAD", m, "v0.99.0")
	require.NoError(t, err)
	_, err = proxy.Add(repo, "HEAD", m, "v0.99.0")
	require.NoError(t, err)

	versionDir := filepath.Join(proxy.Dir, "github.com", "grafana", "ai-sdk", "middleware", "logger", "@v")
	list, err := os.ReadFile(filepath.Join(versionDir, "list"))
	require.NoError(t, err)
	assert.Equal(t, "v0.99.0\n", string(list))
	for _, name := range []string{"v0.99.0.mod", "v0.99.0.info", "v0.99.0.zip"} {
		assert.FileExists(t, filepath.Join(versionDir, name))
	}

	archive, err := zip.OpenReader(filepath.Join(versionDir, "v0.99.0.zip"))
	require.NoError(t, err)
	defer func() { _ = archive.Close() }()
	var names []string
	for _, file := range archive.File {
		names = append(names, file.Name)
	}
	prefix := m.Path + "@v0.99.0/"
	assert.Contains(t, names, prefix+"LICENSE")
	assert.Contains(t, names, prefix+"go.mod")
	for _, name := range names {
		assert.True(t, strings.HasPrefix(name, prefix), "entry %s outside the module prefix", name)
	}
	assert.Equal(t, "file://"+filepath.ToSlash(proxy.Dir), proxy.URL())
}

func TestProxyAdd_ExcludesNestedModules(t *testing.T) {
	repo := repositoryRoot(t)
	proxy := Proxy{Dir: t.TempDir()}
	_, err := proxy.Add(repo, "HEAD", Module{Dir: ".", Path: "github.com/grafana/ai-sdk"}, "v0.99.0")
	require.NoError(t, err)

	archive, err := zip.OpenReader(filepath.Join(proxy.Dir, "github.com", "grafana", "ai-sdk", "@v", "v0.99.0.zip"))
	require.NoError(t, err)
	defer func() { _ = archive.Close() }()
	for _, file := range archive.File {
		name := strings.TrimPrefix(file.Name, "github.com/grafana/ai-sdk@v0.99.0/")
		for _, nested := range []string{"providers/openai/", "ai-gateway/", "release/candidate/", "test/workspace/"} {
			assert.False(t, strings.HasPrefix(name, nested), "root archive includes nested module file %s", name)
		}
	}
}

func TestProxyAdd_UnknownRevision(t *testing.T) {
	_, err := Proxy{Dir: t.TempDir()}.Add(repositoryRoot(t), "refs/tags/does-not-exist", Module{Dir: ".", Path: "github.com/grafana/ai-sdk"}, "v0.99.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "archiving")
}
