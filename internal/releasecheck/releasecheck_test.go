package releasecheck

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repositoryRoot = "../.."

func TestReleasePleaseConfig_PublishedModules(t *testing.T) {
	config, err := LoadConfig(repositoryRoot)
	require.NoError(t, err)
	modules, err := PublishedModules(repositoryRoot)
	require.NoError(t, err)
	require.NotEmpty(t, modules)

	t.Run("every published module is registered", func(t *testing.T) {
		for _, module := range modules {
			assert.Containsf(t, config.Packages, module.Directory,
				"%s is published but missing from release-please-config.json", module.ModulePath)
		}
	})

	t.Run("every registered package is a published module", func(t *testing.T) {
		directories := make(map[string]bool, len(modules))
		for _, module := range modules {
			directories[module.Directory] = true
		}
		for packagePath := range config.Packages {
			assert.Truef(t, directories[packagePath],
				"release-please-config.json registers %q, which is not a published Go module", packagePath)
		}
	})

	t.Run("tags resolve to the module directory", func(t *testing.T) {
		for _, module := range modules {
			tag := config.Tag(module.Directory, "1.2.3")
			assert.Truef(t, ImportableTag(module.Directory, tag),
				"tag %q cannot be resolved by the Go tool for %s", tag, module.ModulePath)
		}
	})

	t.Run("nested components match their directory", func(t *testing.T) {
		for _, module := range modules {
			if module.Directory == RootPackage {
				continue
			}
			assert.Equalf(t, module.Directory, config.Packages[module.Directory].Component,
				"component for %s must equal its repository directory", module.ModulePath)
		}
	})

	t.Run("published modules have no local replacements", func(t *testing.T) {
		for _, module := range modules {
			content, err := os.ReadFile(filepath.Join(repositoryRoot, module.Directory, "go.mod"))
			require.NoError(t, err)
			assert.Falsef(t, HasLocalReplacement(content),
				"%s contains a local replace directive and cannot be published", module.ModulePath)
		}
	})
}

func TestReleasePleaseConfig_Settings(t *testing.T) {
	config, err := LoadConfig(repositoryRoot)
	require.NoError(t, err)

	assert.Equal(t, "go", config.ReleaseType)
	assert.Equal(t, "/", config.TagSeparator, "Go requires <directory>/vX.Y.Z tags for nested modules")
	assert.Equal(t, "prerelease", config.Versioning)
	require.NotNil(t, config.Prerelease)
	assert.True(t, *config.Prerelease, "prerelease versions must be published as prereleases")

	for packagePath, packageConfig := range config.Packages {
		// The release-please config schema only accepts prerelease-type per
		// package, so every module has to name the channel itself.
		assert.Equalf(t, "alpha", packageConfig.PrereleaseType,
			"%q must declare the shared prerelease channel", packagePath)
	}

	root, exists := config.Packages[RootPackage]
	require.True(t, exists, "the root module must be registered")
	require.NotNil(t, root.IncludeComponentInTag)
	assert.False(t, *root.IncludeComponentInTag, "the root module is tagged vX.Y.Z without a component")
}

func TestReleasePleaseConfig_RootExcludesNestedModules(t *testing.T) {
	config, err := LoadConfig(repositoryRoot)
	require.NoError(t, err)
	modules, err := PublishedModules(repositoryRoot)
	require.NoError(t, err)

	excluded := make(map[string]bool, len(config.Packages[RootPackage].ExcludePaths))
	for _, path := range config.Packages[RootPackage].ExcludePaths {
		excluded[path] = true
	}

	for _, module := range modules {
		if module.Directory == RootPackage {
			continue
		}
		parent := filepath.Dir(module.Directory)
		assert.Truef(t, excluded[module.Directory] || excluded[parent],
			"a change to %s must not bump the root module: add %q to the root exclude-paths",
			module.ModulePath, parent)
	}
}

func TestReleasePleaseManifest(t *testing.T) {
	config, err := LoadConfig(repositoryRoot)
	require.NoError(t, err)
	manifest, err := LoadManifest(repositoryRoot)
	require.NoError(t, err)

	t.Run("only registered packages are tracked", func(t *testing.T) {
		for packagePath := range manifest {
			assert.Containsf(t, config.Packages, packagePath,
				"the manifest tracks %q, which is not a configured package", packagePath)
		}
	})

	t.Run("untracked packages declare an initial version", func(t *testing.T) {
		for packagePath, packageConfig := range config.Packages {
			if _, tracked := manifest[packagePath]; tracked {
				continue
			}
			assert.NotEmptyf(t, packageConfig.InitialVersion,
				"%q has never been released and needs an initial-version", packagePath)
		}
	})
}

func TestTag(t *testing.T) {
	config, err := LoadConfig(repositoryRoot)
	require.NoError(t, err)

	tests := []struct {
		packagePath string
		version     string
		want        string
	}{
		{RootPackage, "0.1.0-alpha.2", "v0.1.0-alpha.2"},
		{"providers/openai", "0.1.0-alpha.1", "providers/openai/v0.1.0-alpha.1"},
		{"middleware/logger", "1.0.0", "middleware/logger/v1.0.0"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, config.Tag(tc.packagePath, tc.version))
		})
	}
}

func TestHasLocalReplacement(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"no replacement", "module example.com/a\n\nrequire example.com/b v1.0.0\n", false},
		{"single line", "replace example.com/b => ../b\n", true},
		{"block", "replace (\n\texample.com/b => ./vendor/b\n)\n", true},
		{"remote replacement", "replace example.com/b => example.com/c v1.0.0\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, HasLocalReplacement([]byte(tc.content)))
		})
	}
}
