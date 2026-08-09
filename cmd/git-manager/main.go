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
	"github.com/Odilhao/git-manager/internal/scheduler"
	"github.com/Odilhao/git-manager/internal/status"
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
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "install":
		return runInstall(args[1:], stdout, stderr)
	case "uninstall":
		return runUninstall(args[1:], stdout, stderr)
	case "add":
		return runAdd(args[1:], stdout, stderr)
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

// runStatus reports per-repo drift without changing anything: it runs the
// same plan sync would apply (sync.Run with DryRun: true) and reshapes the
// result as a drift report rather than an apply report.
func runStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "path to the git-manager TOML config file (required)")
	jsonOut := fs.Bool("json", false, "report as JSON instead of human-readable text")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *configPath == "" {
		fmt.Fprintln(stderr, "git-manager status: -config is required")
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "git-manager status: %v\n", err)
		return 2
	}

	syncReport := sync.Run(context.Background(), gitcli.NewClient(), cfg, sync.Options{DryRun: true})
	report := status.FromSyncReport(syncReport)

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(stderr, "git-manager status: encode report: %v\n", err)
			return 2
		}
	} else {
		printStatusReport(stdout, report)
	}

	if report.Drifted {
		return 1
	}
	return 0
}

func printStatusReport(w io.Writer, report status.Report) {
	for _, r := range report.Repos {
		if r.Error != "" {
			fmt.Fprintf(w, "FAIL %s: %s\n", r.Name, r.Error)
			continue
		}
		if !r.Drifted {
			fmt.Fprintf(w, "in sync   %s (%s)\n", r.Name, r.Path)
			continue
		}
		fmt.Fprintf(w, "drift     %s (%s)\n", r.Name, r.Path)
		if r.Cloned {
			fmt.Fprintln(w, "       not cloned")
		}
		for _, c := range r.Remotes.Added {
			fmt.Fprintf(w, "       remote missing: %s -> %s\n", c.Name, c.URL)
		}
		for _, c := range r.Remotes.Updated {
			fmt.Fprintf(w, "       remote stale:   %s -> %s\n", c.Name, c.URL)
		}
		for _, c := range r.Remotes.Removed {
			fmt.Fprintf(w, "       remote undeclared: %s\n", c.Name)
		}
		for _, m := range r.Identity.Mismatched {
			fmt.Fprintf(w, "       identity mismatch: %s = %s\n", m.Key, m.Value)
		}
	}
	fmt.Fprintf(w, "%d repo(s), %d error(s)\n", len(report.Repos), report.ErrorCount)
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

// runInstall installs the git-manager scheduler (systemd/launchd).
func runInstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dryRun := fs.Bool("dry-run", false, "report what would change without applying it")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	result, err := scheduler.Install(context.Background(), scheduler.InstallOptions{DryRun: *dryRun})
	if err != nil {
		fmt.Fprintf(stderr, "git-manager install: %v\n", err)
		return 1
	}

	for _, action := range result.Actions {
		fmt.Fprintln(stdout, action)
	}

	return 0
}

// runUninstall uninstalls the git-manager scheduler (systemd/launchd).
func runUninstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dryRun := fs.Bool("dry-run", false, "report what would change without applying it")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	result, err := scheduler.Uninstall(context.Background(), scheduler.UninstallOptions{DryRun: *dryRun})
	if err != nil {
		fmt.Fprintf(stderr, "git-manager uninstall: %v\n", err)
		return 1
	}

	for _, action := range result.Actions {
		fmt.Fprintln(stdout, action)
	}

	return 0
}
