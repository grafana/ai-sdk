package modulecheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const canonicalURL = "https://github.com/grafana/ai-sdk.git"

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type Pin struct {
	Consumer string
	Module   string
	Version  string
	Commit   string
}

type moduleVersion struct {
	Path    string
	Version string
	Main    bool
	Replace json.RawMessage
}

func goEnv() []string {
	return append(os.Environ(), "GOWORK=off", "GOPROXY=https://proxy.golang.org", "GOPRIVATE=", "GONOPROXY=", "GONOSUMDB=", "GOFLAGS=-mod=readonly")
}

func declaredAndSelected(repo string, consumer Module, published map[string]Module) ([]moduleVersion, error) {
	root := filepath.Join(repo, filepath.FromSlash(consumer.Root))
	data, err := command(root, goEnv(), "go", "mod", "edit", "-json")
	if err != nil {
		return nil, err
	}
	graph, err := command(root, goEnv(), "go", "list", "-m", "-json", "all")
	if err != nil {
		return nil, fmt.Errorf("selected graph for %s: %w", consumer.Path, err)
	}
	return collectVersions(consumer, published, data, graph)
}

func collectVersions(consumer Module, published map[string]Module, manifestJSON, graphJSON []byte) ([]moduleVersion, error) {
	var manifest struct {
		Require []struct {
			Path    string
			Version string
		}
		Replace []json.RawMessage
	}
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		return nil, fmt.Errorf("decoding %s/go.mod: %w", consumer.Root, err)
	}
	if len(manifest.Replace) > 0 {
		return nil, fmt.Errorf("%s contains replace directives", consumer.Path)
	}
	var versions []moduleVersion
	for _, requirement := range manifest.Require {
		if !internalModule(requirement.Path) {
			continue
		}
		if _, ok := published[requirement.Path]; !ok {
			return nil, fmt.Errorf("%s requires unknown internal module %s", consumer.Path, requirement.Path)
		}
		versions = append(versions, moduleVersion{Path: requirement.Path, Version: requirement.Version})
	}
	dec := json.NewDecoder(bytes.NewReader(graphJSON))
	for dec.More() {
		var selected moduleVersion
		if err := dec.Decode(&selected); err != nil {
			return nil, fmt.Errorf("decoding selected graph for %s: %w", consumer.Path, err)
		}
		if !internalModule(selected.Path) || selected.Main {
			continue
		}
		if _, ok := published[selected.Path]; !ok {
			return nil, fmt.Errorf("%s selects unknown internal module %s", consumer.Path, selected.Path)
		}
		if len(selected.Replace) > 0 {
			return nil, fmt.Errorf("%s selected replacement for %s", consumer.Path, selected.Path)
		}
		versions = append(versions, selected)
	}
	return versions, nil
}

func internalModule(path string) bool {
	return path == modulePrefix || strings.HasPrefix(path, modulePrefix+"/")
}

type moduleDownload struct {
	Path    string
	Version string
	Error   string
	Origin  struct {
		VCS    string
		URL    string
		Subdir string
		Hash   string
	}
}

func resolvePin(repo string, module Module, version string) (string, error) {
	return resolvePinWithEnv(repo, module, version, goEnv())
}

func resolvePinWithEnv(repo string, module Module, version string, env []string) (string, error) {
	output, err := command(repo, env, "go", "mod", "download", "-json", module.Path+"@"+version)
	if err != nil {
		return "", err
	}
	var result moduleDownload
	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("decoding module metadata: %w", err)
	}
	return pinCommit(module, version, result)
}

func pinCommit(module Module, version string, result moduleDownload) (string, error) {
	wantSubdir := ""
	if module.Root != "." {
		wantSubdir = module.Root
	}
	if result.Error != "" || result.Path != module.Path || result.Version != version || result.Origin.VCS != "git" ||
		strings.TrimSuffix(result.Origin.URL, ".git") != strings.TrimSuffix(canonicalURL, ".git") ||
		result.Origin.Subdir != wantSubdir || !commitPattern.MatchString(result.Origin.Hash) {
		return "", fmt.Errorf("unverifiable origin for %s@%s: %s", module.Path, version, result.Error)
	}
	return result.Origin.Hash, nil
}

func verifyHistory(remote string, pins []Pin) (resultErr error) {
	dir, err := os.MkdirTemp("", "ai-sdk-main-*")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("removing temporary repository: %w", err)
		}
	}()
	if _, err := command(dir, nil, "git", "init", "--bare", "-q"); err != nil {
		return err
	}
	if _, err := command(dir, nil, "git", "fetch", "--no-tags", remote, "refs/heads/main"); err != nil {
		return fmt.Errorf("fetching canonical main: %w", err)
	}
	anchor, err := command(dir, nil, "git", "rev-parse", "FETCH_HEAD")
	if err != nil {
		return err
	}
	checked := make(map[string]bool)
	for _, pin := range pins {
		hash := pin.Commit
		if checked[hash] {
			continue
		}
		checked[hash] = true
		if !commitPattern.MatchString(hash) {
			return fmt.Errorf("unverifiable commit %q", hash)
		}
		if _, err := command(dir, nil, "git", "cat-file", "-e", hash+"^{commit}"); err != nil {
			if _, err := command(dir, nil, "git", "fetch", "--no-tags", remote, hash); err != nil {
				return fmt.Errorf("%s requires %s@%s: fetching %s from canonical repository: %w", pin.Consumer, pin.Module, pin.Version, hash, err)
			}
		}
		if _, err := command(dir, nil, "git", "merge-base", "--is-ancestor", hash, strings.TrimSpace(string(anchor))); err != nil {
			return fmt.Errorf("%s requires %s@%s (%s): not verified as an ancestor of canonical main: %w", pin.Consumer, pin.Module, pin.Version, hash, err)
		}
	}
	return nil
}

func CheckPins(repo string) ([]Pin, error) {
	pins, err := InventoryPins(repo)
	if err != nil {
		return nil, err
	}
	if err := verifyHistory(canonicalURL, pins); err != nil {
		return nil, fmt.Errorf("verifying published internal pins: %w", err)
	}
	return pins, nil
}

func InventoryPins(repo string) ([]Pin, error) {
	modules, err := Discover(repo)
	if err != nil {
		return nil, err
	}
	published := make(map[string]Module)
	for _, module := range modules {
		if module.Published {
			published[module.Path] = module
		}
	}
	var pins []Pin
	seen := make(map[string]bool)
	resolved := make(map[string]string)
	for _, consumer := range modules {
		if !consumer.Published {
			continue
		}
		versions, err := declaredAndSelected(repo, consumer, published)
		if err != nil {
			return nil, err
		}
		for _, version := range versions {
			key := version.Path + "@" + version.Version
			hash, ok := resolved[key]
			if !ok {
				hash, err = resolvePin(repo, published[version.Path], version.Version)
				if err != nil {
					return nil, fmt.Errorf("%s requires %s: %w", consumer.Path, key, err)
				}
				resolved[key] = hash
			}
			identity := consumer.Path + " " + key
			if !seen[identity] {
				pins = append(pins, Pin{Consumer: consumer.Path, Module: version.Path, Version: version.Version, Commit: hash})
				seen[identity] = true
			}
		}
	}
	slices.SortFunc(pins, func(a, b Pin) int {
		return strings.Compare(a.Consumer+" "+a.Module+" "+a.Version, b.Consumer+" "+b.Module+" "+b.Version)
	})
	return pins, nil
}
