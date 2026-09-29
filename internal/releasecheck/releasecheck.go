// Package releasecheck validates the release-please component inventory.
//
// release-please owns version calculation, changelog generation, tagging, and
// GitHub Releases. Library publication readiness is validated separately from
// this configuration inventory and from Gateway application readiness.
package releasecheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// CorePath is the module path of the root module.
const CorePath = "github.com/grafana/ai-sdk"

// RootPackage is the release-please package key for the root module.
const RootPackage = "."

// Config is the subset of release-please-config.json this repository asserts on.
type Config struct {
	ReleaseType           string                   `json:"release-type"`
	IncludeComponentInTag *bool                    `json:"include-component-in-tag"`
	IncludeVInTag         *bool                    `json:"include-v-in-tag"`
	TagSeparator          string                   `json:"tag-separator"`
	Versioning            string                   `json:"versioning"`
	Prerelease            *bool                    `json:"prerelease"`
	Packages              map[string]PackageConfig `json:"packages"`
}

// PackageConfig is the per-package release-please configuration.
type PackageConfig struct {
	Component             string   `json:"component"`
	IncludeComponentInTag *bool    `json:"include-component-in-tag"`
	PrereleaseType        string   `json:"prerelease-type"`
	InitialVersion        string   `json:"initial-version"`
	ExcludePaths          []string `json:"exclude-paths"`
}

// Module is a release component with a Go module build boundary.
type Module struct {
	Directory   string
	ModulePath  string
	Application bool
}

// LoadConfig reads the release-please configuration from the repository root.
func LoadConfig(root string) (Config, error) {
	content, err := os.ReadFile(filepath.Join(root, "release-please-config.json"))
	if err != nil {
		return Config{}, fmt.Errorf("releasecheck: reading release-please config: %w", err)
	}
	var config Config
	if err := json.Unmarshal(content, &config); err != nil {
		return Config{}, fmt.Errorf("releasecheck: decoding release-please config: %w", err)
	}
	return config, nil
}

// LoadManifest reads the release-please version manifest.
func LoadManifest(root string) (map[string]string, error) {
	content, err := os.ReadFile(filepath.Join(root, ".release-please-manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("releasecheck: reading release-please manifest: %w", err)
	}
	manifest := make(map[string]string)
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("releasecheck: decoding release-please manifest: %w", err)
	}
	return manifest, nil
}

var moduleDeclaration = regexp.MustCompile(`(?m)^module[ \t]+(\S+)`)

// ReleaseComponents discovers library modules and the Gateway application.
// Example programs and test harnesses are excluded.
func ReleaseComponents(root string) ([]Module, error) {
	var modules []Module
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != "go.mod" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		match := moduleDeclaration.FindSubmatch(content)
		if match == nil {
			return fmt.Errorf("releasecheck: %s has no module declaration", path)
		}
		modulePath := string(match[1])
		if strings.HasPrefix(modulePath, CorePath+"/examples/") ||
			strings.HasPrefix(modulePath, CorePath+"/test/") {
			return nil
		}
		directory, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		modules = append(modules, Module{Directory: filepath.ToSlash(directory), ModulePath: modulePath, Application: modulePath == CorePath+"/ai-gateway"})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Directory < modules[j].Directory })
	return modules, nil
}

// Tag renders the Git tag release-please generates for a package version.
func (config Config) Tag(packagePath, version string) string {
	packageConfig := config.Packages[packagePath]
	includeComponent := boolOr(packageConfig.IncludeComponentInTag, boolOr(config.IncludeComponentInTag, true))
	includeV := boolOr(config.IncludeVInTag, true)

	tag := version
	if includeV {
		tag = "v" + version
	}
	if !includeComponent || packageConfig.Component == "" {
		return tag
	}
	separator := config.TagSeparator
	if separator == "" {
		separator = "-"
	}
	return packageConfig.Component + separator + tag
}

// ImportableTag reports whether a tag lets the Go tool resolve a module
// directory. Root modules use "vX.Y.Z"; nested modules use "<directory>/vX.Y.Z".
func ImportableTag(directory, tag string) bool {
	if directory == RootPackage {
		return strings.HasPrefix(tag, "v")
	}
	return strings.HasPrefix(tag, directory+"/v")
}

var localReplacement = regexp.MustCompile(`(?m)=>[ \t]*(\.{1,2}/|/)`)

// HasLocalReplacement reports whether a go.mod redirects a dependency to the
// local filesystem, which cannot be resolved by module proxy users.
func HasLocalReplacement(content []byte) bool {
	return localReplacement.Match(content)
}

func boolOr(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
