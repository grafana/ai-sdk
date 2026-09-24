package modulecheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const modulePrefix = "github.com/grafana/ai-sdk"

type Module struct {
	Root      string
	Path      string
	Published bool
}

func command(dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, bytes.TrimSpace(output))
	}
	return output, nil
}

func Discover(repo string) ([]Module, error) {
	files, err := command(repo, nil, "git", "ls-files", "-z", "--", "go.mod", "*/go.mod")
	if err != nil {
		return nil, err
	}
	var modules []Module
	seen := make(map[string]bool)
	for _, file := range bytes.Split(bytes.TrimSuffix(files, []byte{0}), []byte{0}) {
		if len(file) == 0 {
			continue
		}
		rel := filepath.FromSlash(string(file))
		if filepath.Base(rel) != "go.mod" {
			continue
		}
		if _, err := os.Stat(filepath.Join(repo, rel)); err != nil {
			return nil, fmt.Errorf("tracked module %s: %w", rel, err)
		}
		data, err := command(repo, append(os.Environ(), "GOWORK=off"), "go", "mod", "edit", "-json", rel)
		if err != nil {
			return nil, err
		}
		var mod struct{ Module struct{ Path string } }
		if err := json.Unmarshal(data, &mod); err != nil {
			return nil, fmt.Errorf("decoding %s: %w", rel, err)
		}
		root := filepath.ToSlash(filepath.Dir(rel))
		if root == "." {
			root = "."
		}
		want := modulePrefix
		if root != "." {
			want += "/" + root
		}
		if mod.Module.Path != want || seen[mod.Module.Path] {
			return nil, fmt.Errorf("invalid or duplicate module path in %s: %q (expected %q)", rel, mod.Module.Path, want)
		}
		seen[mod.Module.Path] = true
		localOnly := strings.HasPrefix(root, "examples/") || strings.HasPrefix(root, "test/")
		modules = append(modules, Module{Root: root, Path: mod.Module.Path, Published: !localOnly})
	}
	if !seen[modulePrefix] {
		return nil, fmt.Errorf("missing tracked root go.mod")
	}
	slices.SortFunc(modules, func(a, b Module) int { return strings.Compare(a.Root, b.Root) })
	return modules, nil
}

func Select(modules []Module, selection string) ([]Module, error) {
	if selection == "" {
		return slices.DeleteFunc(slices.Clone(modules), func(m Module) bool { return !m.Published }), nil
	}
	for _, module := range modules {
		if selection == module.Path || selection == module.Root {
			if module.Published {
				return []Module{module}, nil
			}
			break
		}
	}
	return nil, fmt.Errorf("unknown or local-only published module %q", selection)
}
