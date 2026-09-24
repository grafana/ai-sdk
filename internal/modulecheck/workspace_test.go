package modulecheck

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayWorkspace_CandidateSourceVsPinnedVersion(t *testing.T) {
	repo := t.TempDir()
	root := filepath.Join(repo, "sdk")
	gateway := filepath.Join(repo, "gateway")
	proxy := filepath.Join(repo, "proxy")
	cache := filepath.Join(repo, "cache")
	moduleDir := filepath.Join(proxy, "github.com/grafana/ai-sdk/@v")
	for _, dir := range []string{root, gateway, moduleDir, cache} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(cache, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				_ = os.Chmod(path, 0o755)
			}
			return nil
		})
	})
	write := func(path, data string) {
		t.Helper()
		require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
	}
	rootMod := "module " + modulePrefix + "\n\ngo 1.26.3\n"
	write(filepath.Join(root, "go.mod"), rootMod)
	write(filepath.Join(root, "marker.go"), "package aisdk\nconst Marker = \"candidate\"\n")
	write(filepath.Join(gateway, "go.mod"), "module "+modulePrefix+"/ai-gateway\n\ngo 1.26.3\n\nrequire "+modulePrefix+" v0.0.1\n")
	write(filepath.Join(gateway, "main.go"), "package main\nimport (\"fmt\"; sdk \""+modulePrefix+"\")\nfunc main(){fmt.Print(sdk.Marker)}\n")
	write(filepath.Join(repo, "go.gateway.work"), "go 1.26.3\nuse (\n./sdk\n./gateway\n)\n")
	write(filepath.Join(moduleDir, "v0.0.1.mod"), rootMod)
	write(filepath.Join(moduleDir, "v0.0.1.info"), `{"Version":"v0.0.1","Time":"2026-01-01T00:00:00Z","Origin":{"VCS":"git","URL":"https://github.com/grafana/ai-sdk","Hash":"0123456789abcdef0123456789abcdef01234567"}}`)
	archive, err := os.Create(filepath.Join(moduleDir, "v0.0.1.zip"))
	require.NoError(t, err)
	zipper := zip.NewWriter(archive)
	for name, data := range map[string]string{"go.mod": rootMod, "marker.go": "package aisdk\nconst Marker = \"pinned\"\n"} {
		file, err := zipper.Create(modulePrefix + "@v0.0.1/" + name)
		require.NoError(t, err)
		_, err = file.Write([]byte(data))
		require.NoError(t, err)
	}
	require.NoError(t, zipper.Close())
	require.NoError(t, archive.Close())
	env := append(os.Environ(), "GOPROXY=file://"+proxy, "GOSUMDB=off", "GOPRIVATE=", "GONOPROXY=", "GONOSUMDB=", "GOMODCACHE="+cache, "GOWORK=off", "GOFLAGS=-mod=mod")
	commit, err := resolvePinWithEnv(gateway, Module{Root: ".", Path: modulePrefix}, "v0.0.1", env)
	require.NoError(t, err)
	assert.Equal(t, "0123456789abcdef0123456789abcdef01234567", commit)
	build := func(workspace string) string {
		t.Helper()
		output, err := command(gateway, append(env, "GOWORK="+workspace, "GOFLAGS=-mod=readonly"), "go", "run", ".")
		require.NoError(t, err)
		return strings.TrimSpace(string(output))
	}
	assert.Equal(t, "pinned", build("off"))
	assert.Equal(t, "candidate", build(filepath.Join(repo, "go.gateway.work")))
	write(filepath.Join(root, "marker.go"), "package aisdk\nconst Marker = \"changed candidate\"\n")
	assert.Equal(t, "changed candidate", build(filepath.Join(repo, "go.gateway.work")))
	assert.Equal(t, "pinned", build("off"))
}

func gatewaySourceRepo(t *testing.T) string {
	t.Helper()
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(repo, "ai-gateway/go.mod"))
	if os.IsNotExist(err) {
		t.Skip("requires Gateway source")
	}
	require.NoError(t, err)
	return repo
}

func TestGatewayBuild_CandidateSourceVsPinnedVersion(t *testing.T) {
	repo := gatewaySourceRepo(t)
	dir := t.TempDir()
	candidate := filepath.Join(dir, "candidate.go")
	require.NoError(t, os.WriteFile(candidate, []byte("package provider\nvar _ = candidateSourceOnlyMarker\n"), 0o644))
	overlay, err := json.Marshal(struct {
		Replace map[string]string
	}{Replace: map[string]string{filepath.Join(repo, "provider", "doc.go"): candidate}})
	require.NoError(t, err)
	overlayPath := filepath.Join(dir, "overlay.json")
	require.NoError(t, os.WriteFile(overlayPath, overlay, 0o644))

	for _, root := range []string{"ai-gateway/cmd/grafana-ai-gateway", "ai-gateway/test/providerwire-v4/testserver"} {
		t.Run(root, func(t *testing.T) {
			moduleDir := filepath.Join(repo, root)
			build := func(workspace string) ([]byte, error) {
				env := append(os.Environ(), "GOWORK="+workspace, "GOFLAGS=-mod=readonly")
				return command(moduleDir, env, "go", "build", "-overlay", overlayPath, "-o", filepath.Join(dir, "gateway-build"), ".")
			}
			output, err := build("off")
			require.NoError(t, err, string(output))
			_, err = build(filepath.Join(repo, "go.gateway.work"))
			require.ErrorContains(t, err, "candidateSourceOnlyMarker")
		})
	}
}
