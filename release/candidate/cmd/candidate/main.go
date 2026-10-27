// Command candidate generates and checks the first-party requirement files of
// a combined library release pull request.
//
//	candidate generate -dir <worktree> -base <revision>
//	candidate check -repo <repository> -base <revision> -candidate <revision> [-proxy <dir>]
//	candidate proxy -repo <repository> -rev <revision> -out <dir> <module-dir>@<version>...
//
// generate and check print the changed files and the checksums of every
// co-released version as JSON; check can keep its candidate proxy. proxy builds
// archives of the given modules at a revision and prints their checksums.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/release/candidate"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: candidate {generate|check|proxy} [flags]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	upstream := flags.String("upstream", "", "GOPROXY entry after the candidate proxy (default proxy.golang.org)")
	base := flags.String("base", "", "base revision holding the previous manifest")
	var result candidate.Result
	switch args[0] {
	case "generate":
		dir := flags.String("dir", "", "Git worktree holding the release-please updates")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *dir == "" || *base == "" {
			return errors.New("candidate generate: -dir and -base are required")
		}
		modules, err := candidate.Modules(*dir)
		if err != nil {
			return err
		}
		plan, err := candidate.PlanFor(candidate.Worktree{Dir: *dir}, modules, candidate.Revision{Repo: *dir, Rev: *base})
		if err != nil {
			return err
		}
		if result, err = candidate.Generate(*dir, plan, candidate.Options{Upstream: *upstream}); err != nil {
			return err
		}
	case "check":
		repo := flags.String("repo", "", "Git repository")
		rev := flags.String("candidate", "", "candidate revision to verify")
		proxyDir := flags.String("proxy", "", "directory that keeps the candidate proxy")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *repo == "" || *base == "" || *rev == "" {
			return errors.New("candidate check: -repo, -base and -candidate are required")
		}
		var err error
		if result, err = candidate.Check(*repo, *base, *rev, candidate.Options{Upstream: *upstream, ProxyDir: *proxyDir}); err != nil {
			return err
		}
	case "proxy":
		repo := flags.String("repo", "", "Git repository")
		rev := flags.String("rev", "", "revision to archive")
		out := flags.String("out", "", "proxy directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *repo == "" || *rev == "" || *out == "" || flags.NArg() == 0 {
			return errors.New("candidate proxy: -repo, -rev, -out and at least one <module-dir>@<version> are required")
		}
		modules, err := candidate.Modules(*repo)
		if err != nil {
			return err
		}
		result.Checksums = map[string]candidate.Checksums{}
		for _, arg := range flags.Args() {
			dir, version, ok := strings.Cut(arg, "@")
			index := slices.IndexFunc(modules, func(m candidate.Module) bool { return m.Dir == dir })
			if !ok || index < 0 {
				return fmt.Errorf("candidate proxy: %q is not <registered-module-dir>@<version>", arg)
			}
			sums, err := candidate.Proxy{Dir: *out}.Add(candidate.Revision{Repo: *repo, Rev: *rev}, modules[index], version)
			if err != nil {
				return err
			}
			result.Checksums[modules[index].Path+"@"+version] = sums
		}
	default:
		return fmt.Errorf("candidate: unknown command %q", args[0])
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
