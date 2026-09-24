package modulecheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitFixture(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	return strings.TrimSpace(string(output))
}

func TestCollectVersions_DirectAndSelected(t *testing.T) {
	root := Module{Root: ".", Path: modulePrefix, Published: true}
	openai := Module{Root: "providers/openai", Path: modulePrefix + "/providers/openai", Published: true}
	bedrock := Module{Root: "providers/bedrock", Path: modulePrefix + "/providers/bedrock", Published: true}
	published := map[string]Module{root.Path: root, openai.Path: openai, bedrock.Path: bedrock}
	manifest := []byte(`{"Require":[{"Path":"` + root.Path + `","Version":"v0.1.0"},{"Path":"example.com/external","Version":"v1.0.0"}]}`)
	graph := []byte(`{"Path":"` + bedrock.Path + `","Main":true}{"Path":"` + root.Path + `","Version":"v0.2.0"}{"Path":"` + openai.Path + `","Version":"v0.3.0"}`)
	versions, err := collectVersions(bedrock, published, manifest, graph)
	require.NoError(t, err)
	assert.Equal(t, []moduleVersion{
		{Path: root.Path, Version: "v0.1.0"},
		{Path: root.Path, Version: "v0.2.0"},
		{Path: openai.Path, Version: "v0.3.0"},
	}, versions)
	_, err = collectVersions(bedrock, published, []byte(`{"Replace":[{"Old":{"Path":"x"}}]}`), graph)
	require.ErrorContains(t, err, "replace directives")
	_, err = collectVersions(bedrock, published, manifest, []byte(`{"Path":"`+root.Path+`","Version":"v0.2.0","Replace":{"Path":"x"}}`))
	require.ErrorContains(t, err, "selected replacement")
	unknown := modulePrefix + "/providers/unregistered"
	_, err = collectVersions(bedrock, published, []byte(`{"Require":[{"Path":"`+unknown+`","Version":"v0.1.0"}]}`), graph)
	require.ErrorContains(t, err, "requires unknown internal module")
	_, err = collectVersions(bedrock, published, manifest, []byte(`{"Path":"`+unknown+`","Version":"v0.1.0"}`))
	require.ErrorContains(t, err, "selects unknown internal module")
}

func TestPinCommit_OriginMetadata(t *testing.T) {
	module := Module{Root: "providers/openai", Path: modulePrefix + "/providers/openai"}
	version := "v0.1.0"
	valid := moduleDownload{Path: module.Path, Version: version}
	valid.Origin.VCS = "git"
	valid.Origin.URL = canonicalURL
	valid.Origin.Subdir = module.Root
	valid.Origin.Hash = strings.Repeat("a", 40)
	commit, err := pinCommit(module, version, valid)
	require.NoError(t, err)
	assert.Equal(t, valid.Origin.Hash, commit)

	cases := []struct {
		name   string
		change func(*moduleDownload)
	}{
		{"missing origin", func(m *moduleDownload) { m.Origin.Hash = "" }},
		{"wrong module", func(m *moduleDownload) { m.Path = modulePrefix }},
		{"wrong version", func(m *moduleDownload) { m.Version = "v0.2.0" }},
		{"wrong repository", func(m *moduleDownload) { m.Origin.URL = "https://github.com/other/ai-sdk.git" }},
		{"wrong VCS", func(m *moduleDownload) { m.Origin.VCS = "hg" }},
		{"wrong subdirectory", func(m *moduleDownload) { m.Origin.Subdir = "providers/bedrock" }},
		{"abbreviated commit", func(m *moduleDownload) { m.Origin.Hash = "aaaaaaaaaaaa" }},
		{"download error", func(m *moduleDownload) { m.Error = "not found" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metadata := valid
			tc.change(&metadata)
			_, err := pinCommit(module, version, metadata)
			require.ErrorContains(t, err, "unverifiable origin")
		})
	}
}

func TestVerifyHistory_CanonicalAncestry(t *testing.T) {
	canonical := t.TempDir()
	gitFixture(t, canonical, "init", "-b", "main")
	gitFixture(t, canonical, "config", "user.email", "test@example.com")
	gitFixture(t, canonical, "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(canonical, "payload"), []byte("merged"), 0o644))
	gitFixture(t, canonical, "add", "payload")
	gitFixture(t, canonical, "commit", "-m", "merged")
	merged := gitFixture(t, canonical, "rev-parse", "HEAD")
	gitFixture(t, canonical, "tag", "v0.1.0")
	gitFixture(t, canonical, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(canonical, "payload"), []byte("unmerged"), 0o644))
	gitFixture(t, canonical, "commit", "-am", "branch only")
	unmerged := gitFixture(t, canonical, "rev-parse", "HEAD")
	gitFixture(t, canonical, "tag", "v0.2.0")
	gitFixture(t, canonical, "checkout", "main")
	gitFixture(t, canonical, "checkout", "-b", "synthetic")
	gitFixture(t, canonical, "merge", "--no-ff", "feature", "-m", "synthetic PR merge")
	synthetic := gitFixture(t, canonical, "rev-parse", "HEAD")
	gitFixture(t, canonical, "checkout", "main")

	shallow := t.TempDir()
	gitFixture(t, shallow, "clone", "--depth=1", "file://"+canonical, "fork")
	fork := filepath.Join(shallow, "fork")
	assert.Equal(t, "true", gitFixture(t, fork, "rev-parse", "--is-shallow-repository"))
	gitFixture(t, fork, "config", "user.email", "fork@example.com")
	gitFixture(t, fork, "config", "user.name", "Fork")
	gitFixture(t, fork, "fetch", "origin", "feature")
	gitFixture(t, fork, "merge", "--no-ff", "FETCH_HEAD", "-m", "fork main includes branch")
	gitFixture(t, fork, "merge-base", "--is-ancestor", unmerged, "HEAD")

	cases := []struct {
		name   string
		hash   string
		accept bool
	}{
		{"merged pseudo-version", merged, true},
		{"merged tag commit", gitFixture(t, canonical, "rev-list", "-n", "1", "v0.1.0"), true},
		{"branch-only commit", unmerged, false},
		{"unmerged tag commit", gitFixture(t, canonical, "rev-list", "-n", "1", "v0.2.0"), false},
		{"synthetic PR merge", synthetic, false},
		{"unverifiable revision", strings.Repeat("f", 40), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyHistory(canonical, []Pin{{Consumer: modulePrefix + "/ai-gateway", Module: modulePrefix, Version: "v0.1.0", Commit: tc.hash}})
			if tc.accept {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "ai-gateway requires")
			}
		})
	}
}
