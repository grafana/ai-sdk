package release

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// Check validates release intent, tag shapes, and module publishability.
func Check(ctx context.Context, root string, registry Registry, runner Runner) error {
	if _, err := LoadFragments(root, registry); err != nil {
		return err
	}
	for _, module := range registry.Modules {
		if module.Directory != "." {
			content, err := os.ReadFile(filepath.Join(root, module.Directory, "go.mod"))
			if err != nil {
				return fmt.Errorf("release: reading %s/go.mod: %w", module.Directory, err)
			}
			if localReplace(content) {
				return fmt.Errorf("release: module %q contains a local filesystem replace directive", module.ID)
			}
		}
		if _, _, err := CurrentVersion(ctx, root, module, runner); err != nil {
			return err
		}
	}
	return nil
}

var (
	replaceLine      = regexp.MustCompile(`^replace\s+\S+(?:\s+[^=\s]+)?\s*=>\s*(?:\.\.?/|/|[A-Za-z]:[\\/])`)
	replaceBlockLine = regexp.MustCompile(`^\S+(?:\s+[^=\s]+)?\s*=>\s*(?:\.\.?/|/|[A-Za-z]:[\\/])`)
)

func localReplace(content []byte) bool {
	inReplaceBlock := false
	for _, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.SplitN(rawLine, "//", 2)[0])
		if line == "" {
			continue
		}
		if line == "replace (" {
			inReplaceBlock = true
			continue
		}
		if inReplaceBlock && line == ")" {
			inReplaceBlock = false
			continue
		}
		if replaceLine.MatchString(line) || (inReplaceBlock && replaceBlockLine.MatchString(line)) {
			return true
		}
	}
	return false
}

// Prepare writes reviewed changelog, module requirement, fragment, and plan changes.
func Prepare(ctx context.Context, root string, registry Registry, runner Runner, options PlanOptions, date time.Time) (Plan, error) {
	if err := Check(ctx, root, registry, runner); err != nil {
		return Plan{}, err
	}
	fragments, err := LoadFragments(root, registry)
	if err != nil {
		return Plan{}, err
	}
	plan, err := BuildPlan(ctx, root, registry, fragments, runner, options)
	if err != nil {
		return Plan{}, err
	}

	updates := make(map[string][]byte)
	for _, item := range plan.Releases {
		path := filepath.Join(root, item.Changelog)
		content, err := os.ReadFile(path)
		if err != nil {
			return Plan{}, fmt.Errorf("release: reading changelog for %s: %w", item.ModuleID, err)
		}
		updates[path] = updateChangelog(content, item, date)
	}

	coreVersion := ""
	for _, item := range plan.Releases {
		if item.ModuleID == "core" {
			coreVersion = item.Version
			break
		}
	}
	if coreVersion != "" {
		for _, item := range plan.Releases {
			if item.ModuleID == "core" {
				continue
			}
			path := filepath.Join(root, item.Directory, "go.mod")
			content, err := os.ReadFile(path)
			if err != nil {
				return Plan{}, fmt.Errorf("release: reading %s/go.mod: %w", item.ModuleID, err)
			}
			updated, err := updateRootRequirement(content, coreVersion)
			if err != nil {
				return Plan{}, fmt.Errorf("release: updating %s: %w", item.ModuleID, err)
			}
			updates[path] = updated
		}
	}
	planJSON, err := plan.JSON()
	if err != nil {
		return Plan{}, err
	}
	updates[filepath.Join(root, planPath)] = planJSON
	deferred := make(map[string]DeferredFragment, len(plan.DeferredFragments))
	selectedFragments := make(map[string]bool, len(plan.Fragments))
	for _, file := range plan.Fragments {
		selectedFragments[file] = true
	}
	for _, fragment := range plan.DeferredFragments {
		deferred[fragment.File] = fragment
		if selectedFragments[fragment.File] {
			updates[filepath.Join(root, changesPath, fragment.File)] = renderFragment(fragment.Bumps, fragment.Summary)
		}
	}

	paths := make([]string, 0, len(updates))
	for path := range updates {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := writeAtomically(path, updates[path]); err != nil {
			return Plan{}, err
		}
	}
	for _, file := range plan.Fragments {
		if _, remains := deferred[file]; remains {
			continue
		}
		if err := os.Remove(filepath.Join(root, changesPath, file)); err != nil {
			return Plan{}, fmt.Errorf("release: consuming fragment %s: %w", file, err)
		}
	}
	return plan, nil
}

func updateChangelog(content []byte, item Release, date time.Time) []byte {
	var section strings.Builder
	fmt.Fprintf(&section, "## %s - %s\n\n", item.Version, date.Format("2006-01-02"))
	for _, entry := range item.Entries {
		fmt.Fprintf(&section, "- %s\n", strings.ReplaceAll(strings.TrimSpace(entry), "\n", " "))
	}
	section.WriteByte('\n')

	text := strings.TrimRight(string(content), "\n") + "\n"
	firstNewline := strings.IndexByte(text, '\n')
	if firstNewline < 0 {
		return []byte(text + "\n" + section.String())
	}
	return []byte(text[:firstNewline+1] + "\n" + section.String() + strings.TrimLeft(text[firstNewline+1:], "\n"))
}

var rootRequirement = regexp.MustCompile(`(?m)^([ \t]*(?:require[ \t]+)?github\.com/grafana/ai-sdk[ \t]+)v[^ \t\r\n]+(.*)$`)

func updateRootRequirement(content []byte, version string) ([]byte, error) {
	if !rootRequirement.Match(content) {
		return nil, errors.New("root module requirement not found")
	}
	return rootRequirement.ReplaceAll(content, []byte("${1}"+version+"${2}")), nil
}

func writeAtomically(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("release: creating directory for %s: %w", path, err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".release-*")
	if err != nil {
		return fmt.Errorf("release: creating temporary file for %s: %w", path, err)
	}
	tempPath := file.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("release: writing temporary file for %s: %w", path, err)
	}
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return fmt.Errorf("release: setting permissions for %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("release: closing temporary file for %s: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("release: replacing %s: %w", path, err)
	}
	return nil
}

// PublishOptions controls whether publication is a dry run or externally mutating.
type PublishOptions struct {
	Confirm bool
}

// PublishAction describes one checked or performed publication step.
type PublishAction struct {
	Module string
	Action string
}

// Publish validates and performs dependency-ordered module publication.
func Publish(ctx context.Context, root string, registry Registry, runner Runner, options PublishOptions) ([]PublishAction, error) {
	if err := Check(ctx, root, registry, runner); err != nil {
		return nil, err
	}
	plan, err := LoadPlan(root)
	if err != nil {
		return nil, err
	}
	if err := validatePreparedPlan(registry, plan); err != nil {
		return nil, err
	}
	if err := validatePreparedFiles(root, plan); err != nil {
		return nil, err
	}
	status, err := runner.Run(ctx, root, nil, "git", "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("release: checking worktree: %w", err)
	}
	if strings.TrimSpace(status) != "" {
		return nil, errors.New("release: publication requires a clean worktree")
	}
	head, err := runner.Run(ctx, root, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("release: resolving HEAD: %w", err)
	}
	head = strings.TrimSpace(head)

	var actions []PublishAction
	for _, item := range plan.Releases {
		tagState, err := inspectTag(ctx, root, item.Tag, head, runner)
		if err != nil {
			return actions, err
		}
		if tagState == tagMissing {
			module := moduleByID(registry, item.ModuleID)
			current, found, err := CurrentVersion(ctx, root, module, runner)
			if err != nil {
				return actions, err
			}
			if (item.Previous == "") != !found || (found && current.String() != item.Previous) {
				currentValue := "(unreleased)"
				if found {
					currentValue = current.String()
				}
				return actions, fmt.Errorf("release: %s current version is %s, prepared from %s", item.ModuleID, currentValue, preparedPrevious(item))
			}
		}
		testAction := fmt.Sprintf("test %s with GOWORK=off", item.Directory)
		if item.Directory == "." {
			testAction = "test root module"
		}
		actions = append(actions, PublishAction{Module: item.ModuleID, Action: testAction})
		if !options.Confirm {
			actions = append(actions,
				PublishAction{Module: item.ModuleID, Action: "create and push tag " + item.Tag},
				PublishAction{Module: item.ModuleID, Action: "create or verify GitHub Release " + item.Tag},
			)
			continue
		}

		environment := []string(nil)
		if item.Directory != "." {
			environment = []string{"GOWORK=off"}
		}
		if _, err := runner.Run(ctx, filepath.Join(root, item.Directory), environment, "go", "test", "./..."); err != nil {
			return actions, fmt.Errorf("release: testing %s: %w", item.ModuleID, err)
		}
		if tagState == tagMissing {
			if _, err := runner.Run(ctx, root, nil, "git", "tag", "-a", item.Tag, "-m", releaseTitle(item)); err != nil {
				return actions, fmt.Errorf("release: creating tag %s: %w", item.Tag, err)
			}
		}
		if _, err := runner.Run(ctx, root, nil, "git", "push", "origin", "refs/tags/"+item.Tag); err != nil {
			return actions, fmt.Errorf("release: pushing tag %s: %w", item.Tag, err)
		}
		actions = append(actions, PublishAction{Module: item.ModuleID, Action: "tag ready " + item.Tag})
		if _, err := runner.Run(ctx, root, nil, "gh", "release", "view", item.Tag, "--json", "url"); err != nil {
			notes := releaseNotes(item)
			if _, createErr := runner.Run(ctx, root, nil, "gh", "release", "create", item.Tag, "--title", releaseTitle(item), "--notes", notes); createErr != nil {
				return actions, fmt.Errorf("release: creating GitHub Release %s: %w", item.Tag, createErr)
			}
		}
		actions = append(actions, PublishAction{Module: item.ModuleID, Action: "GitHub Release ready " + item.Tag})
	}
	return actions, nil
}

func validatePreparedPlan(registry Registry, plan Plan) error {
	if plan.SchemaVersion != 1 || len(plan.Releases) == 0 || len(plan.Fragments) == 0 {
		return errors.New("release: invalid prepared plan")
	}
	digest, err := hex.DecodeString(plan.FragmentDigest)
	if err != nil || len(digest) != sha256.Size {
		return errors.New("release: prepared plan has an invalid fragment digest")
	}
	fragmentSet := make(map[string]bool, len(plan.Fragments))
	for _, file := range plan.Fragments {
		if filepath.Base(file) != file || filepath.Ext(file) != ".md" || fragmentSet[file] {
			return fmt.Errorf("release: prepared plan has invalid fragment %q", file)
		}
		fragmentSet[file] = true
	}
	byID := make(map[string]Module, len(registry.Modules))
	order := make(map[string]int, len(registry.Modules))
	ordered, err := OrderedModules(registry)
	if err != nil {
		return err
	}
	for index, module := range ordered {
		byID[module.ID] = module
		order[module.ID] = index
	}
	if len(plan.SelectedModules) != len(plan.Releases) {
		return errors.New("release: prepared selected modules do not match releases")
	}
	last := -1
	seen := make(map[string]bool)
	referencedFragments := make(map[string]bool)
	for _, item := range plan.Releases {
		module, exists := byID[item.ModuleID]
		if !exists {
			return fmt.Errorf("release: prepared plan names unknown module %q", item.ModuleID)
		}
		if seen[item.ModuleID] {
			return fmt.Errorf("release: prepared plan repeats module %q", item.ModuleID)
		}
		seen[item.ModuleID] = true
		if order[item.ModuleID] <= last {
			return errors.New("release: prepared plan is not in dependency order")
		}
		last = order[item.ModuleID]
		if item.Directory != module.Directory || item.ModulePath != module.ModulePath || item.Changelog != module.Changelog {
			return fmt.Errorf("release: prepared plan metadata for %q does not match registry", item.ModuleID)
		}
		if _, err := ParseBump(string(item.Bump)); err != nil {
			return err
		}
		if len(item.Entries) == 0 || len(item.FragmentFiles) == 0 {
			return fmt.Errorf("release: prepared plan release %q has no changelog intent", item.ModuleID)
		}
		for _, file := range item.FragmentFiles {
			if !fragmentSet[file] {
				return fmt.Errorf("release: prepared plan release %q references unknown fragment %q", item.ModuleID, file)
			}
			referencedFragments[file] = true
		}
		if !slices.Equal(item.Dependencies, module.Dependencies) {
			return fmt.Errorf("release: prepared dependencies for %q do not match registry", item.ModuleID)
		}
		if item.Previous != "" {
			if _, err := ParseVersion(item.Previous); err != nil {
				return err
			}
		}
		version, err := ParseVersion(item.Version)
		if err != nil {
			return err
		}
		expectedTag := version.String()
		if module.TagPrefix != "" {
			expectedTag = module.TagPrefix + "/" + expectedTag
		}
		if item.Tag != expectedTag {
			return fmt.Errorf("release: prepared tag %q must be %q", item.Tag, expectedTag)
		}
	}
	for index, id := range plan.SelectedModules {
		if id != plan.Releases[index].ModuleID {
			return errors.New("release: prepared selected modules are not in release order")
		}
	}
	for file := range fragmentSet {
		if !referencedFragments[file] {
			return fmt.Errorf("release: prepared fragment %q is not referenced by a release", file)
		}
	}
	deferredFiles := make(map[string]bool)
	for _, fragment := range plan.DeferredFragments {
		if filepath.Base(fragment.File) != fragment.File || filepath.Ext(fragment.File) != ".md" || deferredFiles[fragment.File] {
			return fmt.Errorf("release: prepared plan has invalid deferred fragment %q", fragment.File)
		}
		deferredFiles[fragment.File] = true
		if strings.TrimSpace(fragment.Summary) == "" || len(fragment.Bumps) == 0 {
			return fmt.Errorf("release: deferred fragment %q has no release intent", fragment.File)
		}
		for id, bump := range fragment.Bumps {
			if _, exists := byID[id]; !exists {
				return fmt.Errorf("release: deferred fragment %q names unknown module %q", fragment.File, id)
			}
			if seen[id] {
				return fmt.Errorf("release: deferred fragment %q retains selected module %q", fragment.File, id)
			}
			if _, err := ParseBump(string(bump)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePreparedFiles(root string, plan Plan) error {
	coreVersion := ""
	for _, item := range plan.Releases {
		if item.ModuleID == "core" {
			coreVersion = item.Version
		}
		changelog, err := os.ReadFile(filepath.Join(root, item.Changelog))
		if err != nil {
			return fmt.Errorf("release: reading prepared changelog for %s: %w", item.ModuleID, err)
		}
		if !strings.Contains(string(changelog), "## "+item.Version+" - ") {
			return fmt.Errorf("release: changelog for %s is missing %s", item.ModuleID, item.Version)
		}
		for _, entry := range item.Entries {
			bullet := "- " + strings.ReplaceAll(strings.TrimSpace(entry), "\n", " ")
			if !strings.Contains(string(changelog), bullet) {
				return fmt.Errorf("release: changelog for %s is missing prepared entry %q", item.ModuleID, entry)
			}
		}
	}
	if coreVersion != "" {
		for _, item := range plan.Releases {
			if item.ModuleID == "core" {
				continue
			}
			goMod, err := os.ReadFile(filepath.Join(root, item.Directory, "go.mod"))
			if err != nil {
				return fmt.Errorf("release: reading prepared go.mod for %s: %w", item.ModuleID, err)
			}
			expected := "github.com/grafana/ai-sdk " + coreVersion
			if !strings.Contains(string(goMod), expected) {
				return fmt.Errorf("release: %s does not require prepared core version %s", item.ModuleID, coreVersion)
			}
		}
	}
	deferred := make(map[string]DeferredFragment, len(plan.DeferredFragments))
	for _, fragment := range plan.DeferredFragments {
		deferred[fragment.File] = fragment
	}
	for _, file := range plan.Fragments {
		path := filepath.Join(root, changesPath, file)
		if _, remains := deferred[file]; remains {
			continue
		}
		_, err := os.Stat(path)
		if err == nil {
			return fmt.Errorf("release: prepared fragment %s was not consumed", file)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("release: checking consumed fragment %s: %w", file, err)
		}
	}
	for _, remainder := range plan.DeferredFragments {
		content, err := os.ReadFile(filepath.Join(root, changesPath, remainder.File))
		if err != nil {
			return fmt.Errorf("release: reading deferred fragment %s: %w", remainder.File, err)
		}
		expected := renderFragment(remainder.Bumps, remainder.Summary)
		if !bytes.Equal(content, expected) {
			return fmt.Errorf("release: deferred fragment %s does not match prepared plan", remainder.File)
		}
	}
	return nil
}

type tagState int

const (
	tagMissing tagState = iota
	tagAtHead
)

func inspectTag(ctx context.Context, root, tag, head string, runner Runner) (tagState, error) {
	tags, err := runner.Run(ctx, root, nil, "git", "tag", "--list", tag)
	if err != nil {
		return tagMissing, fmt.Errorf("release: checking tag %s: %w", tag, err)
	}
	if strings.TrimSpace(tags) == "" {
		return tagMissing, nil
	}
	output, err := runner.Run(ctx, root, nil, "git", "rev-parse", "--verify", "refs/tags/"+tag+"^{commit}")
	if err != nil {
		return tagMissing, fmt.Errorf("release: resolving tag %s: %w", tag, err)
	}
	commit := strings.TrimSpace(output)
	if commit != head {
		return tagMissing, fmt.Errorf("release: tag %s points to %s, not prepared commit %s", tag, commit, head)
	}
	return tagAtHead, nil
}

func moduleByID(registry Registry, id string) Module {
	for _, module := range registry.Modules {
		if module.ID == id {
			return module
		}
	}
	return Module{}
}

func preparedPrevious(item Release) string {
	if item.Previous == "" {
		return "(unreleased)"
	}
	return item.Previous
}

func releaseTitle(item Release) string {
	if item.ModuleID == "core" {
		return "AI SDK " + item.Version
	}
	return item.ModuleID + " " + item.Version
}

func releaseNotes(item Release) string {
	var notes strings.Builder
	for _, entry := range item.Entries {
		fmt.Fprintf(&notes, "- %s\n", strings.ReplaceAll(strings.TrimSpace(entry), "\n", " "))
	}
	return notes.String()
}

// MarshalActions renders publication actions for maintainers.
func MarshalActions(actions []PublishAction) string {
	var output strings.Builder
	for _, action := range actions {
		fmt.Fprintf(&output, "%-34s %s\n", action.Module, action.Action)
	}
	return output.String()
}

func decodePlan(content []byte) (Plan, error) {
	var plan Plan
	if err := json.Unmarshal(content, &plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}
