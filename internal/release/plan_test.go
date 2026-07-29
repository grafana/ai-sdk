package release

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type runnerFunc func(context.Context, string, []string, string, ...string) (string, error)

func (function runnerFunc) Run(ctx context.Context, directory string, environment []string, name string, args ...string) (string, error) {
	return function(ctx, directory, environment, name, args...)
}

func TestBuildPlan_AggregatesAndOrdersModulesDeterministically(t *testing.T) {
	t.Parallel()
	registry := testRegistry()
	fragments := []Fragment{
		{
			File:    "a.md",
			Content: []byte("a"),
			Bumps:   map[string]Bump{"providers/openai": BumpPatch, "core": BumpPatch},
			Summary: "First change.",
		},
		{
			File:    "b.md",
			Content: []byte("b"),
			Bumps:   map[string]Bump{"core": BumpMinor},
			Summary: "Second change.",
		},
	}
	runner := runnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) (string, error) {
		require.Equal(t, "git", name)
		if strings.Join(args, " ") == "tag --list v*" {
			return "v0.1.0-alpha.1\n", nil
		}
		if strings.Join(args, " ") == "tag --list providers/openai/v*" {
			return "", nil
		}
		return "", errors.New("unexpected command")
	})

	first, err := BuildPlan(context.Background(), "/repo", registry, fragments, runner, PlanOptions{})
	require.NoError(t, err)
	second, err := BuildPlan(context.Background(), "/repo", registry, fragments, runner, PlanOptions{})
	require.NoError(t, err)
	assert.Equal(t, first, second)
	require.Len(t, first.Releases, 2)
	assert.Equal(t, "core", first.Releases[0].ModuleID)
	assert.Equal(t, BumpMinor, first.Releases[0].Bump)
	assert.Equal(t, "v0.2.0-alpha.1", first.Releases[0].Version)
	assert.Equal(t, []string{"First change.", "Second change."}, first.Releases[0].Entries)
	assert.Equal(t, "providers/openai", first.Releases[1].ModuleID)
	assert.Equal(t, "v0.1.0-alpha.1", first.Releases[1].Version)
	assert.Equal(t, "providers/openai/v0.1.0-alpha.1", first.Releases[1].Tag)

	firstJSON, err := first.JSON()
	require.NoError(t, err)
	secondJSON, err := second.JSON()
	require.NoError(t, err)
	assert.Equal(t, firstJSON, secondJSON)
}

func TestBuildPlan_SelectsModulesAndRecordsDeferredIntent(t *testing.T) {
	t.Parallel()
	registry := testRegistry()
	fragments := []Fragment{
		{
			File:    "shared.md",
			Content: []byte("shared"),
			Bumps:   map[string]Bump{"core": BumpMinor, "providers/openai": BumpPatch},
			Summary: "Add continuation support.",
		},
		{
			File:    "core-only.md",
			Content: []byte("core-only"),
			Bumps:   map[string]Bump{"core": BumpPatch},
			Summary: "Fix core retries.",
		},
	}
	runner := runnerFunc(func(_ context.Context, _ string, _ []string, name string, args ...string) (string, error) {
		require.Equal(t, "git", name)
		if strings.Join(args, " ") == "tag --list providers/openai/v*" {
			return "", nil
		}
		return "", errors.New("unexpected command")
	})

	plan, err := BuildPlan(context.Background(), "/repo", registry, fragments, runner, PlanOptions{
		Modules: []string{"providers/openai"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"providers/openai"}, plan.SelectedModules)
	assert.Equal(t, []string{"shared.md"}, plan.Fragments)
	require.Len(t, plan.Releases, 1)
	assert.Equal(t, "providers/openai", plan.Releases[0].ModuleID)
	assert.Equal(t, []string{"Add continuation support."}, plan.Releases[0].Entries)
	assert.Equal(t, []DeferredFragment{
		{
			File:    "core-only.md",
			Bumps:   map[string]Bump{"core": BumpPatch},
			Summary: "Fix core retries.",
		},
		{
			File:    "shared.md",
			Bumps:   map[string]Bump{"core": BumpMinor},
			Summary: "Add continuation support.",
		},
	}, plan.DeferredFragments)
	assert.Contains(t, plan.Text(), "DEFERRED INTENT")
	assert.Contains(t, plan.Text(), "core-only.md")
}

func TestBuildPlan_SelectedModuleRequiresPendingIntent(t *testing.T) {
	t.Parallel()
	registry := testRegistry()
	fragments := []Fragment{{
		File:    "provider.md",
		Content: []byte("provider"),
		Bumps:   map[string]Bump{"providers/openai": BumpPatch},
		Summary: "Fix provider.",
	}}

	_, err := BuildPlan(context.Background(), "/repo", registry, fragments, runnerFunc(func(
		context.Context, string, []string, string, ...string,
	) (string, error) {
		return "", errors.New("runner should not be called")
	}), PlanOptions{Modules: []string{"core"}})
	require.ErrorContains(t, err, `selected module "core" has no pending release intent`)
}

func testRegistry() Registry {
	return Registry{Modules: []Module{
		{
			ID:             "core",
			Directory:      ".",
			ModulePath:     "github.com/grafana/ai-sdk",
			Changelog:      "CHANGELOG.md",
			InitialVersion: "v0.1.0-alpha.1",
		},
		{
			ID:             "providers/openai",
			Directory:      "providers/openai",
			ModulePath:     "github.com/grafana/ai-sdk/providers/openai",
			TagPrefix:      "providers/openai",
			Changelog:      "providers/openai/CHANGELOG.md",
			InitialVersion: "v0.1.0-alpha.1",
			Dependencies:   []string{"core"},
		},
	}}
}
