// Command git-manager keeps local git checkouts in sync with a declarative
// TOML config.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Odilhao/git-manager/internal/config"
	"github.com/Odilhao/git-manager/internal/gitcli"
	"github.com/Odilhao/git-manager/internal/sync"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches to a subcommand and returns the process exit code. It
// takes stdout/stderr as parameters, rather than using os.Stdout/os.Stderr
// directly, so tests can capture output without touching the real process
// streams.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: git-manager <command> [flags]")
		return 1
	}

	switch args[0] {
	case "sync":
		return runSync(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "git-manager: unknown command %q\n", args[0])
		return 1
	}
}

func runSync(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "path to the git-manager TOML config file (required)")
	dryRun := fs.Bool("dry-run", false, "report what would change without applying it")
	overwrite := fs.Bool("overwrite", false, "remove undeclared remotes during reconciliation (alias: --prune)")
	prune := fs.Bool("prune", false, "remove undeclared remotes during reconciliation (alias: --overwrite)")
	jsonOut := fs.Bool("json", false, "report as JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// internal/platform's cross-platform default config path resolution
	// doesn't exist yet, so -config is required rather than defaulted.
	if *configPath == "" {
		fmt.Fprintln(stderr, "git-manager sync: -config is required")
		return 1
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "git-manager sync: %v\n", err)
		return 1
	}

	opts := sync.Options{DryRun: *dryRun, Overwrite: *overwrite || *prune}
	report := sync.Run(context.Background(), gitcli.NewClient(), cfg, opts)

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(stderr, "git-manager sync: encode report: %v\n", err)
			return 1
		}
	} else {
		printReport(stdout, report)
	}

	if report.ErrorCount > 0 {
		return 1
	}
	return 0
}

func printReport(w io.Writer, report sync.Report) {
	verb := "synced"
	if report.DryRun {
		verb = "would sync"
	}
	for _, r := range report.Repos {
		if r.Error != "" {
			fmt.Fprintf(w, "FAIL %s: %s\n", r.Name, r.Error)
			continue
		}
		fmt.Fprintf(w, "OK   %s (%s) [%s]\n", r.Name, r.Path, verb)
		if r.Cloned {
			fmt.Fprintln(w, "       cloned")
		}
		for _, c := range r.Remotes.Added {
			fmt.Fprintf(w, "       remote added:   %s -> %s\n", c.Name, c.URL)
		}
		for _, c := range r.Remotes.Updated {
			fmt.Fprintf(w, "       remote updated: %s -> %s\n", c.Name, c.URL)
		}
		for _, c := range r.Remotes.Removed {
			fmt.Fprintf(w, "       remote removed: %s\n", c.Name)
		}
		for _, w2 := range r.Identity.Written {
			fmt.Fprintf(w, "       identity set:   %s = %s\n", w2.Key, w2.Value)
		}
		for _, f := range r.Fetches {
			fmt.Fprintf(w, "       fetched:        %s (%s)\n", f.Remote, f.Report.Mode)
		}
	}
	fmt.Fprintf(w, "%d repo(s), %d error(s)\n", len(report.Repos), report.ErrorCount)
}
