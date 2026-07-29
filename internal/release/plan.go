package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Runner executes the existing git, go, and gh command-line tools.
type Runner interface {
	Run(ctx context.Context, directory string, environment []string, name string, args ...string) (string, error)
}

// ExecRunner executes commands in the local operating-system environment.
type ExecRunner struct{}

// Run executes a command and returns its combined output.
func (ExecRunner) Run(ctx context.Context, directory string, environment []string, name string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = directory
	command.Env = append(os.Environ(), environment...)
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

// PlanOptions selects modules, prerelease continuation, or stable promotion.
type PlanOptions struct {
	Prerelease string
	Stable     bool
	Modules    []string
}

// Plan is the deterministic, reviewable release transaction record.
type Plan struct {
	SchemaVersion     int                `json:"schemaVersion"`
	FragmentDigest    string             `json:"fragmentDigest"`
	Fragments         []string           `json:"fragments"`
	SelectedModules   []string           `json:"selectedModules"`
	DeferredFragments []DeferredFragment `json:"deferredFragments,omitempty"`
	Releases          []Release          `json:"releases"`
}

// DeferredFragment records the exact release intent left after preparation.
type DeferredFragment struct {
	File    string          `json:"file"`
	Bumps   map[string]Bump `json:"bumps"`
	Summary string          `json:"summary"`
}

// Release describes one module version and tag in a plan.
type Release struct {
	ModuleID      string   `json:"module"`
	Directory     string   `json:"directory"`
	ModulePath    string   `json:"modulePath"`
	Previous      string   `json:"previousVersion,omitempty"`
	Version       string   `json:"version"`
	Tag           string   `json:"tag"`
	Bump          Bump     `json:"bump"`
	Changelog     string   `json:"changelog"`
	Entries       []string `json:"entries"`
	Dependencies  []string `json:"dependencies,omitempty"`
	FragmentFiles []string `json:"fragmentFiles"`
}

type releaseAggregate struct {
	bump      Bump
	entries   []string
	fragments []string
}

// BuildPlan aggregates fragments and calculates dependency-ordered releases.
func BuildPlan(ctx context.Context, root string, registry Registry, fragments []Fragment, runner Runner, options PlanOptions) (Plan, error) {
	if options.Stable && options.Prerelease != "" {
		return Plan{}, errors.New("release: --stable and --prerelease cannot be used together")
	}
	if options.Prerelease != "" && !regexpPrerelease.MatchString(options.Prerelease) {
		return Plan{}, fmt.Errorf("release: invalid prerelease channel %q", options.Prerelease)
	}
	if len(fragments) == 0 {
		return Plan{}, errors.New("release: no pending change fragments")
	}
	fragments = append([]Fragment(nil), fragments...)
	sort.Slice(fragments, func(i, j int) bool { return fragments[i].File < fragments[j].File })

	aggregates := make(map[string]*releaseAggregate)
	for _, fragment := range fragments {
		ids := make([]string, 0, len(fragment.Bumps))
		for id := range fragment.Bumps {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			item := aggregates[id]
			if item == nil {
				item = &releaseAggregate{}
				aggregates[id] = item
			}
			item.bump = HigherBump(item.bump, fragment.Bumps[id])
			item.entries = append(item.entries, fragment.Summary)
			item.fragments = append(item.fragments, fragment.File)
		}
	}

	ordered, err := OrderedModules(registry)
	if err != nil {
		return Plan{}, err
	}
	selected, err := selectedModuleSet(registry, aggregates, options.Modules)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{SchemaVersion: 1}
	hash := sha256.New()
	for _, fragment := range fragments {
		included := false
		deferred := make(map[string]Bump)
		for id, bump := range fragment.Bumps {
			if selected[id] {
				included = true
			} else {
				deferred[id] = bump
			}
		}
		if !included {
			plan.DeferredFragments = append(plan.DeferredFragments, DeferredFragment{
				File:    fragment.File,
				Bumps:   deferred,
				Summary: fragment.Summary,
			})
			continue
		}
		plan.Fragments = append(plan.Fragments, fragment.File)
		hash.Write([]byte(fragment.File))
		hash.Write([]byte{0})
		hash.Write(fragment.Content)
		hash.Write([]byte{0})
		if len(deferred) > 0 {
			plan.DeferredFragments = append(plan.DeferredFragments, DeferredFragment{
				File:    fragment.File,
				Bumps:   deferred,
				Summary: fragment.Summary,
			})
		}
	}
	plan.FragmentDigest = hex.EncodeToString(hash.Sum(nil))

	for _, module := range ordered {
		item := aggregates[module.ID]
		if item == nil || !selected[module.ID] {
			continue
		}
		plan.SelectedModules = append(plan.SelectedModules, module.ID)
		current, found, err := CurrentVersion(ctx, root, module, runner)
		if err != nil {
			return Plan{}, err
		}
		var next Version
		if !found {
			next, err = ParseVersion(module.InitialVersion)
			if err == nil {
				switch {
				case options.Stable:
					next.Prerelease = ""
					next.Sequence = 0
				case options.Prerelease != "":
					next.Prerelease = options.Prerelease
					next.Sequence = 1
				}
			}
		} else {
			next, err = NextVersion(current, item.bump, options.Prerelease, options.Stable)
		}
		if err != nil {
			return Plan{}, fmt.Errorf("release: planning %s: %w", module.ID, err)
		}
		tag := next.String()
		if module.TagPrefix != "" {
			tag = module.TagPrefix + "/" + tag
		}
		previous := ""
		if found {
			previous = current.String()
		}
		plan.Releases = append(plan.Releases, Release{
			ModuleID:      module.ID,
			Directory:     module.Directory,
			ModulePath:    module.ModulePath,
			Previous:      previous,
			Version:       next.String(),
			Tag:           tag,
			Bump:          item.bump,
			Changelog:     module.Changelog,
			Entries:       append([]string(nil), item.entries...),
			Dependencies:  append([]string(nil), module.Dependencies...),
			FragmentFiles: append([]string(nil), item.fragments...),
		})
	}
	return plan, nil
}

func selectedModuleSet(registry Registry, aggregates map[string]*releaseAggregate, requested []string) (map[string]bool, error) {
	selected := make(map[string]bool)
	if len(requested) == 0 {
		for id := range aggregates {
			selected[id] = true
		}
		return selected, nil
	}
	known := make(map[string]bool, len(registry.Modules))
	for _, module := range registry.Modules {
		known[module.ID] = true
	}
	for _, id := range requested {
		if !known[id] {
			return nil, fmt.Errorf("release: unknown selected module %q", id)
		}
		if selected[id] {
			return nil, fmt.Errorf("release: module %q was selected more than once", id)
		}
		if aggregates[id] == nil {
			return nil, fmt.Errorf("release: selected module %q has no pending release intent", id)
		}
		selected[id] = true
	}
	return selected, nil
}

var regexpPrerelease = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z-]*$`)

// CurrentVersion discovers a module's highest version from matching Git tags.
func CurrentVersion(ctx context.Context, root string, module Module, runner Runner) (Version, bool, error) {
	pattern := "v*"
	prefix := ""
	if module.TagPrefix != "" {
		prefix = module.TagPrefix + "/"
		pattern = prefix + "v*"
	}
	output, err := runner.Run(ctx, root, nil, "git", "tag", "--list", pattern)
	if err != nil {
		return Version{}, false, fmt.Errorf("release: listing tags for %s: %w", module.ID, err)
	}
	var current Version
	found := false
	for _, tag := range strings.Fields(output) {
		value := strings.TrimPrefix(tag, prefix)
		version, err := ParseVersion(value)
		if err != nil {
			return Version{}, false, fmt.Errorf("release: invalid tag %q for %s: %w", tag, module.ID, err)
		}
		if !found || version.Compare(current) > 0 {
			current = version
			found = true
		}
	}
	return current, found, nil
}

// JSON returns deterministic indented plan JSON.
func (plan Plan) JSON() ([]byte, error) {
	content, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("release: encoding plan: %w", err)
	}
	return append(content, '\n'), nil
}

// Text returns a human-readable release summary.
func (plan Plan) Text() string {
	var output strings.Builder
	fmt.Fprintf(&output, "%-34s %-22s %-36s %s\n", "MODULE", "PREVIOUS", "NEXT", "TAG")
	for _, item := range plan.Releases {
		previous := item.Previous
		if previous == "" {
			previous = "(unreleased)"
		}
		fmt.Fprintf(&output, "%-34s %-22s %-36s %s\n", item.ModuleID, previous, item.Version, item.Tag)
		for _, entry := range item.Entries {
			fmt.Fprintf(&output, "  - %s\n", strings.ReplaceAll(entry, "\n", " "))
		}
	}
	if len(plan.DeferredFragments) > 0 {
		output.WriteString("\nDEFERRED INTENT\n")
		for _, fragment := range plan.DeferredFragments {
			ids := make([]string, 0, len(fragment.Bumps))
			for id := range fragment.Bumps {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			fmt.Fprintf(&output, "%s\n", fragment.File)
			for _, id := range ids {
				fmt.Fprintf(&output, "  - %-32s %s\n", id, fragment.Bumps[id])
			}
		}
	}
	return output.String()
}

// LoadPlan reads the prepared release transaction record.
func LoadPlan(root string) (Plan, error) {
	content, err := os.ReadFile(filepath.Join(root, planPath))
	if err != nil {
		return Plan{}, fmt.Errorf("release: reading prepared plan: %w", err)
	}
	var plan Plan
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return Plan{}, fmt.Errorf("release: decoding prepared plan: %w", err)
	}
	if plan.SchemaVersion != 1 || len(plan.Releases) == 0 {
		return Plan{}, errors.New("release: invalid prepared plan")
	}
	return plan, nil
}
