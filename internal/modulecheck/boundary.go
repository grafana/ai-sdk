package modulecheck

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func CheckSourceImports(repo string, modules []Module) error {
	files, err := command(repo, nil, "git", "ls-files", "-z", "--", "*.go")
	if err != nil {
		return err
	}
	for _, file := range bytes.Split(bytes.TrimSuffix(files, []byte{0}), []byte{0}) {
		if len(file) == 0 {
			continue
		}
		name := string(file)
		if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(name))); err != nil {
			return fmt.Errorf("tracked source %s: %w", name, err)
		}
		owner, ok := Owner(modules, name)
		if !ok {
			return fmt.Errorf("no module owns %s", name)
		}
		if owner.Path == modulePrefix+"/ai-gateway" {
			continue
		}
		source, err := parser.ParseFile(token.NewFileSet(), filepath.Join(repo, filepath.FromSlash(name)), nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", name, err)
		}
		for _, imported := range source.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return fmt.Errorf("parsing import in %s: %w", name, err)
			}
			if path == modulePrefix+"/ai-gateway" || strings.HasPrefix(path, modulePrefix+"/ai-gateway/") {
				return fmt.Errorf("%s (%s) imports the AI Gateway module %s", name, owner.Path, path)
			}
		}
	}
	return nil
}
