package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	releasetool "github.com/grafana/ai-sdk/internal/release"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Print(helpText)
		return nil
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	registry, err := releasetool.LoadRegistry(root)
	if err != nil {
		return err
	}
	runner := releasetool.ExecRunner{}

	switch args[0] {
	case "change":
		return runChange(root, registry, args[1:])
	case "check":
		flags := flag.NewFlagSet("check", flag.ContinueOnError)
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("release: check does not accept positional arguments")
		}
		if err := releasetool.Check(ctx, root, registry, runner); err != nil {
			return err
		}
		fmt.Println("release metadata is valid")
		return nil
	case "plan":
		return runPlan(ctx, root, registry, runner, args[1:])
	case "prepare":
		return runPrepare(ctx, root, registry, runner, args[1:])
	case "publish":
		return runPublish(ctx, root, registry, runner, args[1:])
	default:
		return usageError()
	}
}

type bumpFlags map[string]releasetool.Bump

func (values bumpFlags) String() string {
	var items []string
	for module, bump := range values {
		items = append(items, module+"="+string(bump))
	}
	sort.Strings(items)
	return strings.Join(items, ",")
}

func (values bumpFlags) Set(value string) error {
	module, bumpValue, found := strings.Cut(value, "=")
	if !found || module == "" {
		return errors.New("bump must have the form module=patch|minor|major")
	}
	bump, err := releasetool.ParseBump(bumpValue)
	if err != nil {
		return err
	}
	values[module] = bump
	return nil
}

type moduleFlags []string

func (values *moduleFlags) String() string {
	return strings.Join(*values, ",")
}

func (values *moduleFlags) Set(value string) error {
	if value == "" {
		return errors.New("module id cannot be empty")
	}
	*values = append(*values, value)
	return nil
}

func runChange(root string, registry releasetool.Registry, args []string) error {
	flags := flag.NewFlagSet("change", flag.ContinueOnError)
	name := flags.String("name", "", "fragment file name without extension")
	summary := flags.String("summary", "", "changelog entry")
	bumps := make(bumpFlags)
	flags.Var(bumps, "bump", "module=patch|minor|major; repeat for multiple modules")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("release: change does not accept positional arguments")
	}
	path, err := releasetool.CreateFragment(root, *name, *summary, bumps, registry)
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func planFlags(name string, args []string) (releasetool.PlanOptions, bool, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	prerelease := flags.String("prerelease", "", "named prerelease channel, such as alpha")
	stable := flags.Bool("stable", false, "promote prerelease versions to their stable base")
	jsonOutput := flags.Bool("json", false, "print machine-readable JSON")
	var modules moduleFlags
	flags.Var(&modules, "module", "release only this module; repeat for multiple modules")
	if err := flags.Parse(args); err != nil {
		return releasetool.PlanOptions{}, false, err
	}
	if flags.NArg() != 0 {
		return releasetool.PlanOptions{}, false, fmt.Errorf("release: %s does not accept positional arguments", name)
	}
	return releasetool.PlanOptions{
		Prerelease: *prerelease,
		Stable:     *stable,
		Modules:    append([]string(nil), modules...),
	}, *jsonOutput, nil
}

func runPlan(ctx context.Context, root string, registry releasetool.Registry, runner releasetool.Runner, args []string) error {
	options, jsonOutput, err := planFlags("plan", args)
	if err != nil {
		return err
	}
	if err := releasetool.Check(ctx, root, registry, runner); err != nil {
		return err
	}
	fragments, err := releasetool.LoadFragments(root, registry)
	if err != nil {
		return err
	}
	plan, err := releasetool.BuildPlan(ctx, root, registry, fragments, runner, options)
	if err != nil {
		return err
	}
	if jsonOutput {
		content, err := plan.JSON()
		if err != nil {
			return err
		}
		fmt.Print(string(content))
	} else {
		fmt.Print(plan.Text())
	}
	return nil
}

func runPrepare(ctx context.Context, root string, registry releasetool.Registry, runner releasetool.Runner, args []string) error {
	options, jsonOutput, err := planFlags("prepare", args)
	if err != nil {
		return err
	}
	if jsonOutput {
		return errors.New("release: prepare does not support --json")
	}
	plan, err := releasetool.Prepare(ctx, root, registry, runner, options, time.Now())
	if err != nil {
		return err
	}
	fmt.Print(plan.Text())
	return nil
}

func runPublish(ctx context.Context, root string, registry releasetool.Registry, runner releasetool.Runner, args []string) error {
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	confirm := flags.Bool("confirm", false, "create and push tags and GitHub Releases")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("release: publish does not accept positional arguments")
	}
	actions, err := releasetool.Publish(ctx, root, registry, runner, releasetool.PublishOptions{Confirm: *confirm})
	if err != nil {
		return err
	}
	if !*confirm {
		fmt.Println("dry run; no tags or GitHub Releases were changed")
	}
	fmt.Print(releasetool.MarshalActions(actions))
	return nil
}

func repositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("release: resolving working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "release", "modules.json")); err == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("release: run from the ai-sdk repository")
		}
		directory = parent
	}
}

func usageError() error {
	return errors.New("usage: release <change|check|plan|prepare|publish> [flags]")
}

const helpText = `Manage independent AI SDK Go module releases.

Usage:
  release change --name <name> --summary <text> --bump <module>=<level>
  release check
  release plan [--module <id>...] [--json] [--prerelease <channel> | --stable]
  release prepare [--module <id>...] [--prerelease <channel> | --stable]
  release publish [--confirm]

Publication is a dry run unless --confirm is present.
`
