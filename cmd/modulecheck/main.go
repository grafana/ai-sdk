package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/grafana/ai-sdk/internal/modulecheck"
)

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: modulecheck pins | modules [module-root-or-path]")
	}
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("locating repository: %w", err)
	}
	repo := strings.TrimSpace(string(output))
	switch os.Args[1] {
	case "pins", "inventory":
		if len(os.Args) != 2 {
			return fmt.Errorf("usage: modulecheck %s", os.Args[1])
		}
		check := modulecheck.CheckPins
		if os.Args[1] == "inventory" {
			check = modulecheck.InventoryPins
		}
		pins, err := check(repo)
		if err != nil {
			return err
		}
		for _, pin := range pins {
			fmt.Printf("%s: %s@%s %s\n", pin.Consumer, pin.Module, pin.Version, pin.Commit)
		}
	case "boundary-sources", "boundary-modules":
		if len(os.Args) != 2 {
			return fmt.Errorf("usage: modulecheck %s", os.Args[1])
		}
		modules, err := modulecheck.Discover(repo)
		if err != nil {
			return err
		}
		if os.Args[1] == "boundary-sources" {
			return modulecheck.CheckSourceImports(repo, modules)
		}
		return modulecheck.CheckModuleReferences(repo, modules)
	case "modules", "all-modules":
		if len(os.Args) > 3 || (os.Args[1] == "all-modules" && len(os.Args) != 2) {
			return fmt.Errorf("usage: modulecheck modules [module-root-or-path] | all-modules")
		}
		selection := ""
		if len(os.Args) == 3 {
			selection = os.Args[2]
		}
		modules, err := modulecheck.Discover(repo)
		if err != nil {
			return err
		}
		selected := modules
		if os.Args[1] == "modules" {
			selected, err = modulecheck.Select(modules, selection)
			if err != nil {
				return err
			}
		}
		for _, module := range selected {
			fmt.Printf("%s\t%s\n", module.Root, module.Path)
		}
	default:
		return fmt.Errorf("usage: modulecheck pins | modules [module-root-or-path] | all-modules | boundary-sources")
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
