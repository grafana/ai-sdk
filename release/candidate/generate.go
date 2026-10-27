package candidate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
)

// PublicProxy is the upstream module proxy for prerequisites that are already
// tagged.
const PublicProxy = "https://proxy.golang.org"

var localOnlyPrefixes = []string{"examples/", "release/", "test/"}

// Options configures requirement generation.
type Options struct {
	// Upstream is the GOPROXY entry consulted after the candidate proxy for
	// tagged and third-party modules. Use "off" to forbid network access.
	Upstream string
	// SumDB verifies upstream first-party checksums. Empty uses the Go default;
	// "off" disables verification, which is only meant for offline fixtures.
	SumDB string
}

// Result reports the files a generation changed and the checksums of every
// co-released version, keyed by "path@version".
type Result struct {
	Files     []string
	Checksums map[string]Checksums
}

// Modules lists the tracked first-party modules of the repository at dir,
// excluding local-only examples, release tooling and test modules.
func Modules(dir string) ([]Module, error) {
	out, err := gitOutput(dir, "ls-files", "-z", "--", "go.mod", "*/go.mod")
	if err != nil {
		return nil, fmt.Errorf("candidate: listing modules: %w", err)
	}
	var modules []Module
	for name := range strings.SplitSeq(string(out), "\x00") {
		if name == "" || slices.ContainsFunc(localOnlyPrefixes, func(prefix string) bool { return strings.HasPrefix(name, prefix) }) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return nil, fmt.Errorf("candidate: reading %s: %w", name, err)
		}
		modulePath := modfile.ModulePath(content)
		moduleDir := filepath.ToSlash(filepath.Dir(name))
		want := CorePath
		if moduleDir != "." {
			want += "/" + moduleDir
		}
		if modulePath != want {
			return nil, fmt.Errorf("candidate: %s declares %q, expected %q", name, modulePath, want)
		}
		modules = append(modules, Module{Dir: moduleDir, Path: modulePath})
	}
	slices.SortFunc(modules, func(a, b Module) int { return strings.Compare(a.Dir, b.Dir) })
	return modules, nil
}

// Generate rewrites the first-party requirements of the plan's targets in the
// Git worktree at dir and records their checksums. Released modules are built
// into a candidate proxy in first-party dependency order, so each module is
// tidied against the exact archives of its co-released prerequisites. Every
// run uses a fresh module cache.
func Generate(dir string, plan Plan, options Options) (Result, error) {
	work, err := os.MkdirTemp("", "candidate-generate-")
	if err != nil {
		return Result{}, fmt.Errorf("candidate: creating work directory: %w", err)
	}
	defer func() { _ = removeAll(work) }()
	proxy := Proxy{Dir: filepath.Join(work, "proxy")}
	env := goEnv(work, proxy, options)

	original, err := requirementFiles(dir, plan.Targets)
	if err != nil {
		return Result{}, err
	}
	changed := map[string]bool{}
	for _, target := range plan.Targets {
		name := repoPath(target.Dir, "go.mod")
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return Result{}, fmt.Errorf("candidate: reading %s: %w", name, err)
		}
		rewritten, ok, err := plan.RewriteRequirements(name, content)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), rewritten, 0o644); err != nil {
			return Result{}, fmt.Errorf("candidate: writing %s: %w", name, err)
		}
		changed[target.Dir] = true
	}

	order, err := dependencyOrder(dir, plan)
	if err != nil {
		return Result{}, err
	}
	result := Result{Checksums: map[string]Checksums{}}
	tree := Worktree{Dir: dir}
	for _, target := range order {
		if changed[target.Dir] {
			if err := tidy(dir, target, env); err != nil {
				return Result{}, err
			}
		}
		version, released := plan.Released[target.Path]
		if !released {
			continue
		}
		sums, err := proxy.Add(tree, target, version)
		if err != nil {
			return Result{}, err
		}
		result.Checksums[target.Path+"@"+version] = sums
	}
	if err := verifyUpstream(dir, plan, options); err != nil {
		return Result{}, err
	}

	generated, err := requirementFiles(dir, plan.Targets)
	if err != nil {
		return Result{}, err
	}
	for name, content := range generated {
		if !bytes.Equal(content, original[name]) {
			result.Files = append(result.Files, name)
		}
	}
	slices.Sort(result.Files)
	return result, nil
}

func requirementFiles(dir string, targets []Module) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, target := range targets {
		for _, name := range []string{repoPath(target.Dir, "go.mod"), repoPath(target.Dir, "go.sum")} {
			content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("candidate: reading %s: %w", name, err)
			}
			files[name] = content
		}
	}
	return files, nil
}

func dependencyOrder(dir string, plan Plan) ([]Module, error) {
	requires := map[string][]string{}
	for _, target := range plan.Targets {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(repoPath(target.Dir, "go.mod"))))
		if err != nil {
			return nil, fmt.Errorf("candidate: reading %s go.mod: %w", target.Path, err)
		}
		file, err := modfile.ParseLax(repoPath(target.Dir, "go.mod"), content, nil)
		if err != nil {
			return nil, fmt.Errorf("candidate: parsing %s go.mod: %w", target.Path, err)
		}
		for _, requirement := range file.Require {
			if _, released := plan.Released[requirement.Mod.Path]; released {
				requires[target.Path] = append(requires[target.Path], requirement.Mod.Path)
			}
		}
	}
	byPath := map[string]Module{}
	for _, target := range plan.Targets {
		byPath[target.Path] = target
	}
	var order []Module
	state := map[string]int{}
	var visit func(string) error
	visit = func(modulePath string) error {
		switch state[modulePath] {
		case 1:
			return fmt.Errorf("candidate: first-party requirement cycle through %s", modulePath)
		case 2:
			return nil
		}
		state[modulePath] = 1
		for _, dependency := range requires[modulePath] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[modulePath] = 2
		order = append(order, byPath[modulePath])
		return nil
	}
	for _, target := range plan.Targets {
		if err := visit(target.Path); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func goEnv(work string, proxy Proxy, options Options) []string {
	upstream := options.Upstream
	if upstream == "" {
		upstream = PublicProxy
	}
	env := slices.DeleteFunc(os.Environ(), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		return strings.HasPrefix(name, "GO") && name != "GOROOT" && name != "GOTOOLCHAIN"
	})
	return append(env,
		"GOENV=off",
		"GOWORK=off",
		"GOFLAGS=-mod=mod -modcacherw",
		"GOMODCACHE="+filepath.Join(work, "modcache"),
		"GOCACHE="+filepath.Join(work, "gocache"),
		"GOPATH="+filepath.Join(work, "gopath"),
		"GOPROXY="+proxy.URL()+","+upstream,
		"GOPRIVATE=",
		"GONOPROXY=",
		"GONOSUMDB="+CorePath,
		"GOINSECURE=",
	)
}

func tidy(dir string, target Module, env []string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = filepath.Join(dir, filepath.FromSlash(target.Dir))
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("candidate: tidying %s: %w: %s", target.Path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func verifyUpstream(dir string, plan Plan, options Options) error {
	pending := map[string]Checksums{}
	for _, target := range plan.Targets {
		lines, err := readSums(filepath.Join(dir, filepath.FromSlash(repoPath(target.Dir, "go.sum"))))
		if err != nil {
			return err
		}
		for key, sums := range lines {
			modulePath, version, _ := strings.Cut(key, "@")
			if !firstParty(modulePath) || plan.Released[modulePath] == version {
				continue
			}
			previous := pending[key]
			pending[key] = Checksums{Zip: first(sums.Zip, previous.Zip), GoMod: first(sums.GoMod, previous.GoMod)}
		}
	}
	if len(pending) == 0 {
		return nil
	}
	if options.Upstream == "off" {
		return fmt.Errorf("candidate: cannot verify upstream first-party checksums offline: %s", strings.Join(slices.Sorted(maps.Keys(pending)), ", "))
	}
	work, err := os.MkdirTemp("", "candidate-verify-")
	if err != nil {
		return fmt.Errorf("candidate: creating verification directory: %w", err)
	}
	defer func() { _ = removeAll(work) }()
	upstream := options.Upstream
	if upstream == "" {
		upstream = PublicProxy
	}
	env := goEnv(work, Proxy{}, options)
	env = slices.DeleteFunc(env, func(entry string) bool {
		return strings.HasPrefix(entry, "GOPROXY=") || strings.HasPrefix(entry, "GONOSUMDB=")
	})
	env = append(env, "GOPROXY="+upstream, "GONOSUMDB=")
	if options.SumDB != "" {
		env = append(env, "GOSUMDB="+options.SumDB)
	}
	for _, key := range slices.Sorted(maps.Keys(pending)) {
		cmd := exec.Command("go", "mod", "download", "-json", key)
		cmd.Dir = work
		cmd.Env = env
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		var download struct {
			Sum, GoModSum, Error string
		}
		if decodeErr := json.Unmarshal(out, &download); decodeErr != nil && err == nil {
			err = decodeErr
		}
		if err != nil || download.Error != "" {
			return fmt.Errorf("candidate: verifying %s upstream: %w: %s %s", key, err, download.Error, strings.TrimSpace(stderr.String()))
		}
		want := pending[key]
		if (want.Zip != "" && want.Zip != download.Sum) || (want.GoMod != "" && want.GoMod != download.GoModSum) {
			return fmt.Errorf("candidate: recorded checksum of %s differs from the verified upstream checksum", key)
		}
	}
	return nil
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func readSums(name string) (map[string]Checksums, error) {
	content, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Checksums{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("candidate: reading %s: %w", name, err)
	}
	sums := map[string]Checksums{}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return nil, fmt.Errorf("candidate: malformed line in %s: %q", name, scanner.Text())
		}
		version, goMod := strings.CutSuffix(fields[1], "/go.mod")
		key := fields[0] + "@" + version
		entry := sums[key]
		if goMod {
			entry.GoMod = fields[2]
		} else {
			entry.Zip = fields[2]
		}
		sums[key] = entry
	}
	return sums, scanner.Err()
}

func repoPath(dir, name string) string {
	if dir == "." {
		return name
	}
	return dir + "/" + name
}

func removeAll(dir string) error {
	_ = filepath.WalkDir(dir, func(name string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			_ = os.Chmod(name, 0o755)
		}
		return nil
	})
	return os.RemoveAll(dir)
}
