package modulecheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CheckModuleReferences(repo string, modules []Module) error {
	gatewayPath := modulePrefix + "/ai-gateway"
	gatewayRoot := filepath.Join(repo, "ai-gateway")
	for _, module := range modules {
		if module.Path == gatewayPath {
			continue
		}
		root := filepath.Join(repo, filepath.FromSlash(module.Root))
		data, err := command(root, append(os.Environ(), "GOWORK=off"), "go", "mod", "edit", "-json")
		if err != nil {
			return err
		}
		var manifest struct {
			Require []struct{ Path string }
			Replace []struct {
				Old struct{ Path string }
				New struct{ Path string }
			}
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("decoding %s/go.mod: %w", module.Root, err)
		}
		for _, required := range manifest.Require {
			if gatewayReference(required.Path, gatewayPath) {
				return fmt.Errorf("%s requires the AI Gateway module", module.Path)
			}
		}
		for _, replaced := range manifest.Replace {
			if gatewayReference(replaced.Old.Path, gatewayPath) || gatewayReference(replaced.New.Path, gatewayPath) ||
				localGatewayTarget(root, gatewayRoot, replaced.New.Path) {
				return fmt.Errorf("%s replaces the AI Gateway module or source", module.Path)
			}
		}
	}
	return nil
}

func gatewayReference(path, gateway string) bool {
	return path == gateway || strings.HasPrefix(path, gateway+"/")
}

func localGatewayTarget(root, gatewayRoot, target string) bool {
	if !filepath.IsAbs(target) && !strings.HasPrefix(target, "./") && !strings.HasPrefix(target, "../") {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	clean := filepath.Clean(target)
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		clean = resolved
	}
	gateway := filepath.Clean(gatewayRoot)
	return clean == gateway || strings.HasPrefix(clean, gateway+string(filepath.Separator))
}
