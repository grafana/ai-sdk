package candidate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixtureRepo struct {
	t   *testing.T
	dir string
}

func newFixtureRepo(t *testing.T, rootValue string) fixtureRepo {
	t.Helper()
	repo := fixtureRepo{t: t, dir: t.TempDir()}
	repo.git("init", "-q")
	repo.write("LICENSE", "license\n")
	repo.write("go.mod", "module "+CorePath+"\n\ngo 1.27.3\n")
	repo.write("root.go", "package aisdk\n\nconst Value = \""+rootValue+"\"\n")
	repo.write("providers/openai/go.mod", "module "+CorePath+"/providers/openai\n\ngo 1.27.3\n\nrequire "+CorePath+" v0.1.0\n")
	repo.write("providers/openai/openai.go", "package openai\n\nimport aisdk \""+CorePath+"\"\n\nconst Value = aisdk.Value\n")
	repo.write("providers/bedrock/go.mod", "module "+CorePath+"/providers/bedrock\n\ngo 1.27.3\n\nrequire (\n\t"+CorePath+" v0.1.0\n\t"+CorePath+"/providers/openai v0.1.0\n)\n")
	repo.write("providers/bedrock/bedrock.go", "package bedrock\n\nimport (\n\taisdk \""+CorePath+"\"\n\t\""+CorePath+"/providers/openai\"\n)\n\nconst Value = aisdk.Value + openai.Value\n")
	repo.write("ai-gateway/go.mod", "module "+CorePath+"/ai-gateway\n\ngo 1.27.3\n\nrequire "+CorePath+"/providers/bedrock v0.1.0\n")
	repo.write("ai-gateway/main.go", "package main\n\nimport \""+CorePath+"/providers/bedrock\"\n\nfunc main() { println(bedrock.Value) }\n")
	repo.write("test/workspace/go.mod", "module "+CorePath+"/test/workspace\n\ngo 1.27.3\n")
	repo.git("add", ".")
	repo.git("commit", "-qm", "base")
	return repo
}

func (r fixtureRepo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.dir, "-c", "user.email=t@example.com", "-c", "user.name=T", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(r.t, err, string(out))
}

func (r fixtureRepo) write(name, content string) {
	r.t.Helper()
	target := filepath.Join(r.dir, filepath.FromSlash(name))
	require.NoError(r.t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(r.t, os.WriteFile(target, []byte(content), 0o644))
}

func (r fixtureRepo) read(name string) string {
	r.t.Helper()
	content, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(name)))
	require.NoError(r.t, err)
	return string(content)
}

func fixturePlan(t *testing.T, dir string) Plan {
	t.Helper()
	modules, err := Modules(dir)
	require.NoError(t, err)
	before := map[string]string{".": "0.1.0", "providers/openai": "0.1.0", "providers/bedrock": "0.1.0", GatewayDir: "0.1.0"}
	after := map[string]string{".": "0.2.0", "providers/openai": "0.2.0", "providers/bedrock": "0.2.0", GatewayDir: "0.1.0"}
	plan, err := NewPlan(modules, before, after)
	require.NoError(t, err)
	return plan
}

var offline = Options{Upstream: "off", SumDB: "off"}

func TestModules(t *testing.T) {
	repo := newFixtureRepo(t, "a")
	modules, err := Modules(repo.dir)
	require.NoError(t, err)
	assert.Equal(t, []Module{
		{Dir: ".", Path: CorePath},
		{Dir: GatewayDir, Path: CorePath + "/" + GatewayDir},
		{Dir: "providers/bedrock", Path: CorePath + "/providers/bedrock"},
		{Dir: "providers/openai", Path: CorePath + "/providers/openai"},
	}, modules)

	repo.write("providers/openai/go.mod", "module example.com/wrong\n")
	_, err = Modules(repo.dir)
	assert.ErrorContains(t, err, `declares "example.com/wrong"`)
}

func TestGenerate_ChecksumsMatchRebuiltArchives(t *testing.T) {
	repo := newFixtureRepo(t, "a")
	plan := fixturePlan(t, repo.dir)

	repo.write("stray.txt", "not part of the candidate\n")
	result, err := Generate(repo.dir, plan, offline)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"ai-gateway/go.mod", "ai-gateway/go.sum",
		"providers/bedrock/go.mod", "providers/bedrock/go.sum",
		"providers/openai/go.mod", "providers/openai/go.sum",
	}, result.Files)
	assert.Contains(t, repo.read("providers/bedrock/go.mod"), CorePath+"/providers/openai v0.2.0")
	assert.Contains(t, repo.read("ai-gateway/go.mod"), CorePath+"/providers/bedrock v0.2.0")
	assert.Len(t, result.Checksums, 3)

	repo.git("commit", "-qm", "generated")
	rebuilt := Proxy{Dir: t.TempDir()}
	for _, target := range plan.Targets {
		version, ok := plan.Released[target.Path]
		if !ok {
			continue
		}
		sums, err := rebuilt.Add(Revision{Repo: repo.dir, Rev: "HEAD"}, target, version)
		require.NoError(t, err)
		assert.Equal(t, result.Checksums[target.Path+"@"+version], sums, target.Path)
	}

	recorded := 0
	for _, name := range result.Files {
		if !strings.HasSuffix(name, "go.sum") {
			continue
		}
		sums, err := readSums(filepath.Join(repo.dir, filepath.FromSlash(name)))
		require.NoError(t, err)
		for key, entry := range sums {
			want, ok := result.Checksums[key]
			require.True(t, ok, "%s records unexpected %s", name, key)
			recorded++
			assert.Equal(t, want.GoMod, entry.GoMod, "%s %s go.mod", name, key)
			if entry.Zip != "" {
				assert.Equal(t, want.Zip, entry.Zip, "%s %s zip", name, key)
			}
		}
	}
	assert.Positive(t, recorded)

	cache := t.TempDir()
	build := exec.Command("go", "build", "./...")
	build.Dir = filepath.Join(repo.dir, "ai-gateway")
	build.Env = append(goEnv(cache, rebuilt, offline), "GOFLAGS=-mod=readonly -modcacherw")
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))
	require.NoError(t, removeAll(cache))
}

func TestGenerate_DoesNotReuseCachedArchives(t *testing.T) {
	shared := t.TempDir()
	t.Setenv("GOMODCACHE", shared)
	t.Setenv("GOFLAGS", "-mod=vendor")

	first := newFixtureRepo(t, "first")
	firstResult, err := Generate(first.dir, fixturePlan(t, first.dir), offline)
	require.NoError(t, err)

	second := newFixtureRepo(t, "second")
	secondResult, err := Generate(second.dir, fixturePlan(t, second.dir), offline)
	require.NoError(t, err)

	key := CorePath + "@v0.2.0"
	assert.NotEqual(t, firstResult.Checksums[key], secondResult.Checksums[key])
	second.git("add", ".")
	second.git("commit", "-qm", "generated")
	rebuilt, err := Proxy{Dir: t.TempDir()}.Add(Revision{Repo: second.dir, Rev: "HEAD"}, Module{Dir: ".", Path: CorePath}, "v0.2.0")
	require.NoError(t, err)
	assert.Equal(t, rebuilt, secondResult.Checksums[key])
	entries, err := os.ReadDir(shared)
	require.NoError(t, err)
	assert.Empty(t, entries, "generation wrote to the ambient module cache")
}

func TestGenerate_UntaggedPrerequisite(t *testing.T) {
	repo := newFixtureRepo(t, "a")
	modules, err := Modules(repo.dir)
	require.NoError(t, err)
	plan, err := NewPlan(modules,
		map[string]string{"providers/bedrock": "0.1.0", GatewayDir: "0.1.0"},
		map[string]string{"providers/bedrock": "0.2.0", GatewayDir: "0.1.0"})
	require.NoError(t, err)
	_, err = Generate(repo.dir, plan, offline)
	assert.ErrorIs(t, err, ErrUntaggedPrerequisite)
}

func TestGenerate_OfflineRefusesUnverifiedUpstreamPins(t *testing.T) {
	repo := newFixtureRepo(t, "a")
	repo.write("providers/bedrock/go.sum", CorePath+"/providers/openai v0.1.0/go.mod h1:abc=\n")
	modules, err := Modules(repo.dir)
	require.NoError(t, err)
	plan, err := NewPlan(modules,
		map[string]string{".": "0.1.0", "providers/openai": "0.1.0", "providers/bedrock": "0.1.0", GatewayDir: "0.1.0"},
		map[string]string{".": "0.1.0", "providers/openai": "0.1.0", "providers/bedrock": "0.1.0", GatewayDir: "0.1.0"})
	require.NoError(t, err)
	plan.Targets = []Module{{Dir: "providers/bedrock", Path: CorePath + "/providers/bedrock"}}
	err = verifyUpstream(repo.dir, plan, offline)
	assert.ErrorContains(t, err, "cannot verify upstream first-party checksums offline")
}
