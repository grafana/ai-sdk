// Command candidate generates and checks the first-party requirement files of
// a combined library release pull request.
//
//	candidate generate -dir <worktree> -base <revision>
//	candidate check -repo <repository> -base <revision> -candidate <revision>
//
// Both print the changed files and the checksums of every co-released version
// as JSON.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

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
		return errors.New("usage: candidate {generate|check} [flags]")
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
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *repo == "" || *base == "" || *rev == "" {
			return errors.New("candidate check: -repo, -base and -candidate are required")
		}
		var err error
		if result, err = candidate.Check(*repo, *base, *rev, candidate.Options{Upstream: *upstream}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("candidate: unknown command %q", args[0])
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
