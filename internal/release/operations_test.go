package release

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepare_UpdatesChangelogsRequirementPlanAndFragments(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	registry := testRegistry()
	writeTestFile(t, root, ".changes/change.md", "---\ncore: minor\nproviders/openai: patch\n---\n\nAdd continuation support.\n")
	writeTestFile(t, root, "CHANGELOG.md", "# Changelog\n\nCore history.\n")
	writeTestFile(t, root, "go.mod", "module github.com/grafana/ai-sdk\n")
	writeTestFile(t, root, "providers/openai/CHANGELOG.md", "# Changelog\n\nProvider history.\n")
	writeTestFile(t, root, "providers/openai/go.mod", "module github.com/grafana/ai-sdk/providers/openai\n\nrequire github.com/grafana/ai-sdk v0.1.0-alpha.1\n")
	runner := tagRunner(map[string]string{
		"v*":                  "v0.1.0-alpha.1\n",
		"providers/openai/v*": "",
	})

	plan, err := Prepare(context.Background(), root, registry, runner, PlanOptions{}, time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, plan.Releases, 2)
	assertFileContains(t, root, "CHANGELOG.md", "## v0.2.0-alpha.1 - 2026-07-29")
	assertFileContains(t, root, "providers/openai/CHANGELOG.md", "## v0.1.0-alpha.1 - 2026-07-29")
	assertFileContains(t, root, "providers/openai/go.mod", "github.com/grafana/ai-sdk v0.2.0-alpha.1")
	_, err = os.Stat(filepath.Join(root, ".changes/change.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	content, err := os.ReadFile(filepath.Join(root, planPath))
	require.NoError(t, err)
	decoded, err := decodePlan(content)
	require.NoError(t, err)
	assert.Equal(t, plan, decoded)
}

func TestPrepare_InvalidRootRequirementDoesNotModifyReleaseFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	registry := testRegistry()
	fragment := "---\ncore: minor\nproviders/openai: patch\n---\n\nAdd continuation support.\n"
	coreChangelog := "# Changelog\n\nCore history.\n"
	providerChangelog := "# Changelog\n\nProvider history.\n"
	writeTestFile(t, root, ".changes/change.md", fragment)
	writeTestFile(t, root, "CHANGELOG.md", coreChangelog)
	writeTestFile(t, root, "go.mod", "module github.com/grafana/ai-sdk\n")
	writeTestFile(t, root, "providers/openai/CHANGELOG.md", providerChangelog)
	writeTestFile(t, root, "providers/openai/go.mod", "module github.com/grafana/ai-sdk/providers/openai\n")
	runner := tagRunner(map[string]string{
		"v*":                  "v0.1.0-alpha.1\n",
		"providers/openai/v*": "",
	})

	_, err := Prepare(context.Background(), root, registry, runner, PlanOptions{}, time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC))
	require.ErrorContains(t, err, "root module requirement not found")

	assertFileContains(t, root, ".changes/change.md", fragment)
	assertFileContains(t, root, "CHANGELOG.md", coreChangelog)
	assertFileContains(t, root, "providers/openai/CHANGELOG.md", providerChangelog)
	_, err = os.Stat(filepath.Join(root, planPath))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestPrepare_SelectedModulePreservesDeferredFragmentIntent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	registry := testRegistry()
	writeTestFile(t, root, ".changes/shared.md", "---\ncore: minor\nproviders/openai: patch\n---\n\nAdd continuation support.\n")
	writeTestFile(t, root, ".changes/core-only.md", "---\ncore: patch\n---\n\nFix core retries.\n")
	writeTestFile(t, root, "CHANGELOG.md", "# Changelog\n\nCore history.\n")
	writeTestFile(t, root, "go.mod", "module github.com/grafana/ai-sdk\n")
	writeTestFile(t, root, "providers/openai/CHANGELOG.md", "# Changelog\n\nProvider history.\n")
	writeTestFile(t, root, "providers/openai/go.mod", "module github.com/grafana/ai-sdk/providers/openai\n\nrequire github.com/grafana/ai-sdk v0.1.0-alpha.1\n")
	runner := tagRunner(map[string]string{
		"v*":                  "v0.1.0-alpha.1\n",
		"providers/openai/v*": "",
	})

	plan, err := Prepare(context.Background(), root, registry, runner, PlanOptions{
		Modules: []string{"providers/openai"},
	}, time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, []string{"providers/openai"}, plan.SelectedModules)
	require.Len(t, plan.Releases, 1)
	assert.Equal(t, "providers/openai", plan.Releases[0].ModuleID)
	assertFileContains(t, root, "providers/openai/CHANGELOG.md", "## v0.1.0-alpha.1 - 2026-07-29")
	coreChangelog, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	require.NoError(t, err)
	assert.NotContains(t, string(coreChangelog), "## v0.2.0-alpha.1")
	assertFileContains(t, root, "providers/openai/go.mod", "github.com/grafana/ai-sdk v0.1.0-alpha.1")

	shared, err := os.ReadFile(filepath.Join(root, ".changes/shared.md"))
	require.NoError(t, err)
	assert.Equal(t, "---\ncore: minor\n---\n\nAdd continuation support.\n", string(shared))
	coreOnly, err := os.ReadFile(filepath.Join(root, ".changes/core-only.md"))
	require.NoError(t, err)
	assert.Equal(t, "---\ncore: patch\n---\n\nFix core retries.\n", string(coreOnly))

	require.NoError(t, validatePreparedPlan(registry, plan))
	require.NoError(t, validatePreparedFiles(root, plan))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".changes/shared.md"),
		[]byte("---\ncore: patch\n---\n\nChanged intent.\n"),
		0o644,
	))
	require.ErrorContains(t, validatePreparedFiles(root, plan), "does not match prepared plan")
}

func TestPublish_DryRunDoesNotMutate(t *testing.T) {
	t.Parallel()
	root := publicationRepo(t)
	registry := testRegistry()
	plan := testPlan()
	writePlan(t, root, plan)
	var commands []string
	runner := runnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "git tag --list v*":
			return "v0.1.0-alpha.1\n", nil
		case "git tag --list providers/openai/v*":
			return "", nil
		case "git status --porcelain":
			return "", nil
		case "git rev-parse HEAD":
			return "abc\n", nil
		case "git tag --list providers/openai/v0.1.0-alpha.1":
			return "", nil
		default:
			return "", errors.New("unexpected command: " + command)
		}
	})

	actions, err := Publish(context.Background(), root, registry, runner, PublishOptions{})
	require.NoError(t, err)
	assert.Len(t, actions, 3)
	for _, command := range commands {
		assert.NotContains(t, command, "git push")
		assert.NotContains(t, command, "gh release create")
		assert.NotContains(t, command, "go test")
	}
}

func TestPublish_ConflictingTagStops(t *testing.T) {
	t.Parallel()
	root := publicationRepo(t)
	registry := Registry{Modules: testRegistry().Modules[:1]}
	plan := Plan{SchemaVersion: 1, Releases: []Release{{
		ModuleID: "core", Directory: ".", ModulePath: "github.com/grafana/ai-sdk",
		Previous: "v0.1.0-alpha.1", Version: "v0.1.0-alpha.2", Tag: "v0.1.0-alpha.2",
		Bump: BumpPatch, Changelog: "CHANGELOG.md", Entries: []string{"Fix."},
	}}}
	writePlan(t, root, plan)
	runner := runnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		switch command {
		case "git tag --list v*":
			return "v0.1.0-alpha.1\n", nil
		case "git status --porcelain":
			return "", nil
		case "git rev-parse HEAD":
			return "abc\n", nil
		case "git tag --list v0.1.0-alpha.2":
			return "v0.1.0-alpha.2\n", nil
		case "git rev-parse --verify refs/tags/v0.1.0-alpha.2^{commit}":
			return "def\n", nil
		default:
			return "", errors.New("unexpected command: " + command)
		}
	})

	_, err := Publish(context.Background(), root, registry, runner, PublishOptions{Confirm: true})
	require.ErrorContains(t, err, "points to def")
}

func TestPublish_RetrySkipsExistingTagAtPreparedCommit(t *testing.T) {
	t.Parallel()
	root := publicationRepo(t)
	registry := Registry{Modules: testRegistry().Modules[:1]}
	plan := Plan{SchemaVersion: 1, Releases: []Release{{
		ModuleID: "core", Directory: ".", ModulePath: "github.com/grafana/ai-sdk",
		Previous: "v0.1.0-alpha.1", Version: "v0.1.0-alpha.2", Tag: "v0.1.0-alpha.2",
		Bump: BumpPatch, Changelog: "CHANGELOG.md", Entries: []string{"Fix."},
	}}}
	writePlan(t, root, plan)
	var commands []string
	runner := runnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "git tag --list v*":
			return "v0.1.0-alpha.1\nv0.1.0-alpha.2\n", nil
		case "git status --porcelain":
			return "", nil
		case "git rev-parse HEAD":
			return "abc\n", nil
		case "git tag --list v0.1.0-alpha.2":
			return "v0.1.0-alpha.2\n", nil
		case "git rev-parse --verify refs/tags/v0.1.0-alpha.2^{commit}":
			return "abc\n", nil
		case "go test ./...", "git push origin refs/tags/v0.1.0-alpha.2", "gh release view v0.1.0-alpha.2 --json url":
			return "", nil
		default:
			return "", errors.New("unexpected command: " + command)
		}
	})

	_, err := Publish(context.Background(), root, registry, runner, PublishOptions{Confirm: true})
	require.NoError(t, err)
	assert.NotContains(t, commands, "git tag -a v0.1.0-alpha.2 -m AI SDK v0.1.0-alpha.2")
	assert.Contains(t, commands, "git push origin refs/tags/v0.1.0-alpha.2")
}

func TestPublish_CoreMaySucceedBeforeDependentTestFails(t *testing.T) {
	t.Parallel()
	root := publicationRepo(t)
	registry := testRegistry()
	plan := Plan{SchemaVersion: 1, Releases: []Release{
		{
			ModuleID: "core", Directory: ".", ModulePath: "github.com/grafana/ai-sdk",
			Previous: "v0.1.0-alpha.1", Version: "v0.1.0-alpha.2", Tag: "v0.1.0-alpha.2",
			Bump: BumpPatch, Changelog: "CHANGELOG.md", Entries: []string{"Core fix."},
		},
		{
			ModuleID: "providers/openai", Directory: "providers/openai",
			ModulePath: "github.com/grafana/ai-sdk/providers/openai",
			Version:    "v0.1.0-alpha.1", Tag: "providers/openai/v0.1.0-alpha.1",
			Bump: BumpPatch, Changelog: "providers/openai/CHANGELOG.md",
			Entries: []string{"Provider fix."}, Dependencies: []string{"core"},
		},
	}}
	writePlan(t, root, plan)
	var commands []string
	runner := runnerFunc(func(_ context.Context, directory string, environment []string, name string, args ...string) (string, error) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "git tag --list providers/openai/v*":
			return "", nil
		case "git status --porcelain":
			return "", nil
		case "git rev-parse HEAD":
			return "abc\n", nil
		case "git tag --list v0.1.0-alpha.2", "git tag --list providers/openai/v0.1.0-alpha.1":
			return "", nil
		case "git tag --list v*":
			return "v0.1.0-alpha.1\n", nil
		case "go test ./...":
			if filepath.Base(directory) == "openai" {
				assert.Equal(t, []string{"GOWORK=off"}, environment)
				return "", errors.New("provider test failed")
			}
			return "", nil
		case "git tag -a v0.1.0-alpha.2 -m AI SDK v0.1.0-alpha.2":
			return "", nil
		case "git push origin refs/tags/v0.1.0-alpha.2":
			return "", nil
		case "gh release view v0.1.0-alpha.2 --json url":
			return "{}", nil
		default:
			return "", errors.New("unexpected command: " + command)
		}
	})

	_, err := Publish(context.Background(), root, registry, runner, PublishOptions{Confirm: true})
	require.ErrorContains(t, err, "provider test failed")
	corePush := indexOf(commands, "git push origin refs/tags/v0.1.0-alpha.2")
	providerTest := indexOf(commands, "go test ./...", corePush+1)
	assert.Greater(t, corePush, -1)
	assert.Greater(t, providerTest, corePush)
	assert.NotContains(t, commands, "git push origin refs/tags/providers/openai/v0.1.0-alpha.1")
}

func tagRunner(tags map[string]string) Runner {
	return runnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) (string, error) {
		if name != "git" || len(args) != 3 || args[0] != "tag" || args[1] != "--list" {
			return "", errors.New("unexpected command")
		}
		return tags[args[2]], nil
	})
}

func publicationRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, ".changes/.gitkeep", "")
	writeTestFile(t, root, "go.mod", "module github.com/grafana/ai-sdk\n")
	writeTestFile(t, root, "CHANGELOG.md", "# Changelog\n")
	writeTestFile(t, root, "providers/openai/go.mod", "module github.com/grafana/ai-sdk/providers/openai\n\nrequire github.com/grafana/ai-sdk v0.1.0-alpha.2\n")
	writeTestFile(t, root, "providers/openai/CHANGELOG.md", "# Changelog\n")
	return root
}

func testPlan() Plan {
	return Plan{SchemaVersion: 1, Releases: []Release{{
		ModuleID: "providers/openai", Directory: "providers/openai",
		ModulePath: "github.com/grafana/ai-sdk/providers/openai",
		Version:    "v0.1.0-alpha.1", Tag: "providers/openai/v0.1.0-alpha.1",
		Bump: BumpPatch, Changelog: "providers/openai/CHANGELOG.md",
		Entries: []string{"Fix."}, Dependencies: []string{"core"},
	}}}
}

func writePlan(t *testing.T, root string, plan Plan) {
	t.Helper()
	if plan.SchemaVersion == 0 {
		plan.SchemaVersion = 1
	}
	if plan.FragmentDigest == "" {
		plan.FragmentDigest = strings.Repeat("0", 64)
	}
	if len(plan.Fragments) == 0 {
		plan.Fragments = []string{"change.md"}
	}
	if len(plan.SelectedModules) == 0 {
		for _, item := range plan.Releases {
			plan.SelectedModules = append(plan.SelectedModules, item.ModuleID)
		}
	}
	for index := range plan.Releases {
		if len(plan.Releases[index].FragmentFiles) == 0 {
			plan.Releases[index].FragmentFiles = []string{"change.md"}
		}
		var section strings.Builder
		fmt.Fprintf(&section, "\n## %s - 2026-07-29\n\n", plan.Releases[index].Version)
		for _, entry := range plan.Releases[index].Entries {
			fmt.Fprintf(&section, "- %s\n", strings.ReplaceAll(strings.TrimSpace(entry), "\n", " "))
		}
		path := filepath.Join(root, plan.Releases[index].Changelog)
		existing, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, append(existing, []byte(section.String())...), 0o644))
	}
	content, err := plan.JSON()
	require.NoError(t, err)
	writeTestFile(t, root, planPath, string(content))
}

func writeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func assertFileContains(t *testing.T, root, name, expected string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, name))
	require.NoError(t, err)
	assert.Contains(t, string(content), expected)
}

func indexOf(values []string, target string, start ...int) int {
	from := 0
	if len(start) > 0 {
		from = start[0]
	}
	for index := from; index < len(values); index++ {
		if values[index] == target {
			return index
		}
	}
	return -1
}
