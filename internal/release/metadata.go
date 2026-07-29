package release

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	registryPath = "release/modules.json"
	changesPath  = ".changes"
	planPath     = "release/plan.json"
)

// Registry is the explicit allowlist of independently published Go modules.
type Registry struct {
	Modules []Module `json:"modules"`
}

// Module describes one independently versioned public Go module.
type Module struct {
	ID             string   `json:"id"`
	Directory      string   `json:"directory"`
	ModulePath     string   `json:"modulePath"`
	TagPrefix      string   `json:"tagPrefix"`
	Changelog      string   `json:"changelog"`
	InitialVersion string   `json:"initialVersion"`
	Dependencies   []string `json:"dependencies,omitempty"`
}

// LoadRegistry reads and validates the repository's public-module registry.
func LoadRegistry(root string) (Registry, error) {
	content, err := os.ReadFile(filepath.Join(root, registryPath))
	if err != nil {
		return Registry{}, fmt.Errorf("release: reading module registry: %w", err)
	}
	var registry Registry
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registry); err != nil {
		return Registry{}, fmt.Errorf("release: decoding module registry: %w", err)
	}
	if err := ValidateRegistry(root, registry); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

// ValidateRegistry checks registry identity, files, paths, and dependency order.
func ValidateRegistry(root string, registry Registry) error {
	if len(registry.Modules) == 0 {
		return errors.New("release: module registry is empty")
	}

	ids := make(map[string]bool)
	directories := make(map[string]bool)
	modulePaths := make(map[string]bool)
	rootModules := 0
	for index, module := range registry.Modules {
		if module.ID == "" || module.ModulePath == "" || module.Changelog == "" || module.InitialVersion == "" {
			return fmt.Errorf("release: registry module %d has an empty required field", index)
		}
		if module.Directory == "" || filepath.IsAbs(module.Directory) || filepath.Clean(module.Directory) != module.Directory {
			return fmt.Errorf("release: module %q has invalid directory %q", module.ID, module.Directory)
		}
		if module.Directory == "." {
			rootModules++
			if module.ID != "core" {
				return fmt.Errorf("release: root module id must be %q", "core")
			}
		} else if module.ID != filepath.ToSlash(module.Directory) {
			return fmt.Errorf("release: nested module id %q must match directory %q", module.ID, module.Directory)
		}
		if filepath.IsAbs(module.Changelog) || filepath.Clean(module.Changelog) != module.Changelog {
			return fmt.Errorf("release: module %q has invalid changelog path %q", module.ID, module.Changelog)
		}
		if ids[module.ID] {
			return fmt.Errorf("release: duplicate module id %q", module.ID)
		}
		if directories[module.Directory] {
			return fmt.Errorf("release: duplicate module directory %q", module.Directory)
		}
		if modulePaths[module.ModulePath] {
			return fmt.Errorf("release: duplicate module path %q", module.ModulePath)
		}
		ids[module.ID] = true
		directories[module.Directory] = true
		modulePaths[module.ModulePath] = true

		if _, err := ParseVersion(module.InitialVersion); err != nil {
			return fmt.Errorf("release: module %q initial version: %w", module.ID, err)
		}
		expectedPrefix := module.Directory
		if module.Directory == "." {
			expectedPrefix = ""
		}
		if module.TagPrefix != expectedPrefix {
			return fmt.Errorf("release: module %q tag prefix must be %q", module.ID, expectedPrefix)
		}

		goMod, err := os.ReadFile(filepath.Join(root, module.Directory, "go.mod"))
		if err != nil {
			return fmt.Errorf("release: module %q go.mod: %w", module.ID, err)
		}
		declared := moduleDeclaration(goMod)
		if declared != module.ModulePath {
			return fmt.Errorf("release: module %q declares %q, want %q", module.ID, declared, module.ModulePath)
		}
		if _, err := os.Stat(filepath.Join(root, module.Changelog)); err != nil {
			return fmt.Errorf("release: module %q changelog: %w", module.ID, err)
		}
	}
	if rootModules != 1 {
		return fmt.Errorf("release: registry must contain exactly one root module, found %d", rootModules)
	}
	for _, module := range registry.Modules {
		for _, dependency := range module.Dependencies {
			if !ids[dependency] {
				return fmt.Errorf("release: module %q has unknown dependency %q", module.ID, dependency)
			}
		}
	}
	if _, err := OrderedModules(registry); err != nil {
		return err
	}
	return validateRegistryCoverage(root, directories)
}

func validateRegistryCoverage(root string, registered map[string]bool) error {
	var missing []string
	for _, parent := range []string{"providers", "middleware"} {
		err := filepath.WalkDir(filepath.Join(root, parent), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || entry.Name() != "go.mod" {
				return nil
			}
			directory, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			directory = filepath.ToSlash(directory)
			if !registered[directory] {
				missing = append(missing, directory)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("release: discovering public modules: %w", err)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("release: unregistered public modules: %s", strings.Join(missing, ", "))
	}
	return nil
}

func moduleDeclaration(goMod []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(goMod))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

// OrderedModules returns modules in stable dependency order.
func OrderedModules(registry Registry) ([]Module, error) {
	byID := make(map[string]Module, len(registry.Modules))
	for _, module := range registry.Modules {
		byID[module.ID] = module
	}
	var ordered []Module
	state := make(map[string]int)
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("release: dependency cycle at %q", id)
		case 2:
			return nil
		}
		state[id] = 1
		module := byID[id]
		dependencies := append([]string(nil), module.Dependencies...)
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		ordered = append(ordered, module)
		return nil
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

// Fragment records reviewed release intent from one change.
type Fragment struct {
	File    string
	Bumps   map[string]Bump
	Summary string
	Content []byte
}

var fragmentLine = regexp.MustCompile(`^([a-z0-9][a-z0-9/-]*): (patch|minor|major)$`)

// ParseFragment parses and validates one explicit release-intent fragment.
func ParseFragment(file string, content []byte, registry Registry) (Fragment, error) {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return Fragment{}, fmt.Errorf("release: fragment %s must start with ---", file)
	}
	end := strings.Index(normalized[4:], "\n---\n")
	if end < 0 {
		return Fragment{}, fmt.Errorf("release: fragment %s is missing closing ---", file)
	}
	end += 4
	header := normalized[4:end]
	summary := strings.TrimSpace(normalized[end+5:])
	if summary == "" {
		return Fragment{}, fmt.Errorf("release: fragment %s has an empty summary", file)
	}
	known := make(map[string]bool, len(registry.Modules))
	for _, module := range registry.Modules {
		known[module.ID] = true
	}
	bumps := make(map[string]Bump)
	for _, line := range strings.Split(header, "\n") {
		match := fragmentLine.FindStringSubmatch(line)
		if match == nil {
			return Fragment{}, fmt.Errorf("release: fragment %s has invalid header line %q", file, line)
		}
		if !known[match[1]] {
			return Fragment{}, fmt.Errorf("release: fragment %s names unknown module %q", file, match[1])
		}
		if _, exists := bumps[match[1]]; exists {
			return Fragment{}, fmt.Errorf("release: fragment %s repeats module %q", file, match[1])
		}
		bumps[match[1]] = Bump(match[2])
	}
	if len(bumps) == 0 {
		return Fragment{}, fmt.Errorf("release: fragment %s has no module bumps", file)
	}
	return Fragment{File: file, Bumps: bumps, Summary: summary, Content: content}, nil
}

// LoadFragments loads pending fragments in deterministic filename order.
func LoadFragments(root string, registry Registry) ([]Fragment, error) {
	entries, err := os.ReadDir(filepath.Join(root, changesPath))
	if err != nil {
		return nil, fmt.Errorf("release: reading change fragments: %w", err)
	}
	var fragments []Fragment
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(root, changesPath, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("release: reading fragment %s: %w", entry.Name(), err)
		}
		fragment, err := ParseFragment(entry.Name(), content, registry)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, fragment)
	}
	sort.Slice(fragments, func(i, j int) bool { return fragments[i].File < fragments[j].File })
	return fragments, nil
}

// CreateFragment creates a new release-intent fragment without overwriting.
func CreateFragment(root, name, summary string, bumps map[string]Bump, registry Registry) (string, error) {
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`).MatchString(name) {
		return "", fmt.Errorf("release: fragment name %q must use lowercase letters, digits, and hyphens", name)
	}
	if strings.TrimSpace(summary) == "" {
		return "", errors.New("release: fragment summary is required")
	}
	known := make(map[string]bool, len(registry.Modules))
	for _, module := range registry.Modules {
		known[module.ID] = true
	}
	ids := make([]string, 0, len(bumps))
	for id, bump := range bumps {
		if !known[id] {
			return "", fmt.Errorf("release: unknown module %q", id)
		}
		if _, err := ParseBump(string(bump)); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return "", errors.New("release: at least one --bump module=level is required")
	}
	path := filepath.Join(root, changesPath, name+".md")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("release: creating fragment: %w", err)
	}
	if _, err := file.Write(renderFragment(bumps, summary)); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("release: writing fragment: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("release: closing fragment: %w", err)
	}
	return filepath.ToSlash(filepath.Join(changesPath, name+".md")), nil
}

func renderFragment(bumps map[string]Bump, summary string) []byte {
	ids := make([]string, 0, len(bumps))
	for id := range bumps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var content strings.Builder
	content.WriteString("---\n")
	for _, id := range ids {
		fmt.Fprintf(&content, "%s: %s\n", id, bumps[id])
	}
	content.WriteString("---\n\n")
	content.WriteString(strings.TrimSpace(summary))
	content.WriteByte('\n')
	return []byte(content.String())
}
