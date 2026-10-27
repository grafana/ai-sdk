package candidate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ManifestFile is the release-please version manifest.
const ManifestFile = ".release-please-manifest.json"

// ErrGeneratedMismatch reports requirement files that differ from a fresh
// generation of the same release.
var ErrGeneratedMismatch = errors.New("candidate: requirement files differ from generation")

// PlanFor derives the plan of a candidate tree against the manifest of its
// base revision.
func PlanFor(tree Tree, modules []Module, base Revision) (Plan, error) {
	before, err := readManifest(base)
	if err != nil {
		return Plan{}, err
	}
	after, err := readManifest(tree)
	if err != nil {
		return Plan{}, err
	}
	return NewPlan(modules, before, after)
}

// Check regenerates the requirement files of the release at candidate from
// the base revision's requirement files and fails with ErrGeneratedMismatch
// unless the result is byte-identical to candidate. It returns the checksums of
// every co-released version.
func Check(repo, base, candidate string, options Options) (Result, error) {
	dir, err := os.MkdirTemp("", "candidate-check-")
	if err != nil {
		return Result{}, fmt.Errorf("candidate: creating check directory: %w", err)
	}
	defer func() { _ = removeAll(dir) }()

	revision := Revision{Repo: repo, Rev: candidate}
	if err := snapshot(revision, dir); err != nil {
		return Result{}, err
	}
	modules, err := Modules(dir)
	if err != nil {
		return Result{}, err
	}
	plan, err := PlanFor(revision, modules, Revision{Repo: repo, Rev: base})
	if err != nil {
		return Result{}, err
	}
	for _, target := range plan.Targets {
		for _, name := range []string{repoPath(target.Dir, "go.mod"), repoPath(target.Dir, "go.sum")} {
			if err := restore(repo, base, dir, name); err != nil {
				return Result{}, err
			}
		}
	}
	result, err := Generate(dir, plan, options)
	if err != nil {
		return Result{}, err
	}

	var differing []string
	for _, target := range plan.Targets {
		for _, name := range []string{repoPath(target.Dir, "go.mod"), repoPath(target.Dir, "go.sum")} {
			want, wantErr := revision.readFile(name)
			got, gotErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
			if (wantErr == nil) != (gotErr == nil) || !bytes.Equal(want, got) {
				differing = append(differing, name)
			}
		}
	}
	if len(differing) > 0 {
		return Result{}, fmt.Errorf("%w: %s", ErrGeneratedMismatch, strings.Join(differing, ", "))
	}
	return result, nil
}

func readManifest(tree Tree) (map[string]string, error) {
	content, err := tree.readFile(ManifestFile)
	if err != nil {
		return nil, fmt.Errorf("candidate: reading %s: %w", ManifestFile, err)
	}
	var manifest map[string]string
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("candidate: parsing %s: %w", ManifestFile, err)
	}
	return manifest, nil
}

func snapshot(revision Revision, dir string) error {
	if err := revision.materialize(".", dir); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A", "-f", "."},
		{"-c", "user.email=candidate@localhost", "-c", "user.name=candidate", "-c", "commit.gpgsign=false",
			"commit", "-q", "--allow-empty", "-m", "candidate"},
	} {
		if _, err := gitOutput(dir, args...); err != nil {
			return fmt.Errorf("candidate: preparing check tree: %w", err)
		}
	}
	return nil
}

func restore(repo, base, dir, name string) error {
	target := filepath.Join(dir, filepath.FromSlash(name))
	content, err := gitOutput(repo, "show", base+":"+name)
	if err != nil {
		if exists, existsErr := pathExists(repo, base, name); existsErr != nil {
			return existsErr
		} else if exists {
			return fmt.Errorf("candidate: reading base %s: %w", name, err)
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("candidate: removing %s: %w", name, err)
		}
		return nil
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return fmt.Errorf("candidate: restoring %s: %w", name, err)
	}
	return nil
}

func pathExists(repo, rev, name string) (bool, error) {
	out, err := gitOutput(repo, "ls-tree", "--name-only", rev, "--", name)
	if err != nil {
		return false, fmt.Errorf("candidate: listing %s at %s: %w", name, rev, err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}
