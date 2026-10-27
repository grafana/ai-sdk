package candidate

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	baseManifest     = `{".": "0.1.0", "providers/openai": "0.1.0", "providers/bedrock": "0.1.0", "ai-gateway": "0.1.0"}`
	releasedManifest = `{".": "0.2.0", "providers/openai": "0.2.0", "providers/bedrock": "0.2.0", "ai-gateway": "0.1.0"}`
)

func (r fixtureRepo) head() string {
	r.t.Helper()
	out, err := exec.Command("git", "-C", r.dir, "rev-parse", "HEAD").Output()
	require.NoError(r.t, err)
	return strings.TrimSpace(string(out))
}

func releaseFixture(t *testing.T, edit func(repo fixtureRepo)) (fixtureRepo, string, string) {
	t.Helper()
	repo := newFixtureRepo(t, "a")
	repo.write(ManifestFile, baseManifest)
	repo.git("add", ".")
	repo.git("commit", "-qm", "manifest")
	base := repo.head()

	repo.write(ManifestFile, releasedManifest)
	repo.git("add", ManifestFile)
	modules, err := Modules(repo.dir)
	require.NoError(t, err)
	plan, err := PlanFor(Worktree{Dir: repo.dir}, modules, Revision{Repo: repo.dir, Rev: base})
	require.NoError(t, err)
	_, err = Generate(repo.dir, plan, offline)
	require.NoError(t, err)
	if edit != nil {
		edit(repo)
	}
	repo.git("add", ".")
	repo.git("commit", "-qm", "release")
	return repo, base, repo.head()
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name    string
		edit    func(repo fixtureRepo)
		wantErr string
	}{
		{name: "generated candidate"},
		{
			name: "hand-edited pin",
			edit: func(repo fixtureRepo) {
				content := repo.read("providers/bedrock/go.mod")
				repo.write("providers/bedrock/go.mod", strings.Replace(content, "/providers/openai v0.2.0", "/providers/openai v0.1.0", 1))
			},
			wantErr: "providers/bedrock/go.mod",
		},
		{
			name: "unexpected third-party requirement",
			edit: func(repo fixtureRepo) {
				repo.write("ai-gateway/go.mod", repo.read("ai-gateway/go.mod")+"\nrequire example.com/extra v1.0.0\n")
			},
			wantErr: "ai-gateway/go.mod",
		},
		{
			name: "edited checksum",
			edit: func(repo fixtureRepo) {
				content := repo.read("providers/openai/go.sum")
				repo.write("providers/openai/go.sum", strings.Replace(content, "h1:", "h1:x", 1))
			},
			wantErr: "providers/openai/go.sum",
		},
		{
			name: "release without generated files",
			edit: func(repo fixtureRepo) {
				repo.git("checkout", "HEAD", "--", "providers", "ai-gateway")
				repo.git("clean", "-qfd", "--", "providers", "ai-gateway")
			},
			wantErr: "providers/bedrock/go.mod",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, base, candidate := releaseFixture(t, tc.edit)
			result, err := Check(repo.dir, base, candidate, offline)
			if tc.wantErr != "" {
				require.ErrorIs(t, err, ErrGeneratedMismatch)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, result.Checksums, 3)
		})
	}
}
