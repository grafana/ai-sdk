package candidate

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
)

// CorePath is the root module path; first-party modules share it as prefix.
const CorePath = "github.com/grafana/ai-sdk"

// GatewayDir is the repository directory of the Gateway application, which
// follows co-released library versions without being released by the plan.
const GatewayDir = "ai-gateway"

// ErrUntaggedPrerequisite reports a first-party requirement that is neither
// co-released nor tagged.
var ErrUntaggedPrerequisite = errors.New("candidate: prerequisite is neither co-released nor tagged")

// Plan describes one combined library release: the versions it releases, the
// latest tagged version of every other first-party module, and the modules
// whose requirements it rewrites.
type Plan struct {
	Released map[string]string
	Latest   map[string]string
	Targets  []Module
}

// NewPlan derives a plan from the registered modules and the release manifest
// before and after the release pull request. Manifest keys are repository
// directories and values are versions without the "v" prefix. Components whose
// version changes are released; the previous manifest names the latest tags.
// Gateway is always a target and must not be released by the plan.
func NewPlan(modules []Module, before, after map[string]string) (Plan, error) {
	byDir := make(map[string]Module, len(modules))
	for _, m := range modules {
		byDir[m.Dir] = m
	}
	plan := Plan{Released: map[string]string{}, Latest: map[string]string{}}
	for _, dir := range slices.Sorted(maps.Keys(after)) {
		m, ok := byDir[dir]
		if !ok {
			return Plan{}, fmt.Errorf("candidate: manifest component %q is not a registered module", dir)
		}
		if after[dir] == before[dir] {
			continue
		}
		if dir == GatewayDir {
			return Plan{}, errors.New("candidate: Gateway is released by its own pull request")
		}
		plan.Released[m.Path] = "v" + after[dir]
		plan.Targets = append(plan.Targets, m)
	}
	for dir, version := range before {
		if m, ok := byDir[dir]; ok && dir != GatewayDir {
			plan.Latest[m.Path] = "v" + version
		}
	}
	if gateway, ok := byDir[GatewayDir]; ok {
		plan.Targets = append(plan.Targets, gateway)
	}
	return plan, nil
}

// Version returns the version a target requires for first-party module path:
// the co-released version, otherwise the latest tag.
func (p Plan) Version(path string) (string, error) {
	if version, ok := p.Released[path]; ok {
		return version, nil
	}
	if version, ok := p.Latest[path]; ok {
		return version, nil
	}
	return "", fmt.Errorf("%w: %s", ErrUntaggedPrerequisite, path)
}

// RewriteRequirements sets every first-party requirement of goMod to the
// plan's version and reports whether the manifest changed. Other content is
// preserved.
func (p Plan) RewriteRequirements(name string, goMod []byte) ([]byte, bool, error) {
	file, err := modfile.Parse(name, goMod, nil)
	if err != nil {
		return nil, false, fmt.Errorf("candidate: parsing %s: %w", name, err)
	}
	changed := false
	for _, requirement := range file.Require {
		path := requirement.Mod.Path
		if !firstParty(path) {
			continue
		}
		version, err := p.Version(path)
		if err != nil {
			return nil, false, fmt.Errorf("candidate: %s: %w", name, err)
		}
		if requirement.Mod.Version == version {
			continue
		}
		if err := file.AddRequire(path, version); err != nil {
			return nil, false, fmt.Errorf("candidate: %s: requiring %s@%s: %w", name, path, version, err)
		}
		changed = true
	}
	if !changed {
		return goMod, false, nil
	}
	file.Cleanup()
	formatted, err := file.Format()
	if err != nil {
		return nil, false, fmt.Errorf("candidate: formatting %s: %w", name, err)
	}
	return formatted, true, nil
}

func firstParty(path string) bool {
	return path == CorePath || strings.HasPrefix(path, CorePath+"/")
}
