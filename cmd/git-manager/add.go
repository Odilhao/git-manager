package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Odilhao/git-manager/internal/config"
	"github.com/Odilhao/git-manager/internal/gitcli"
)

// runAdd scaffolds a config entry from a checkout that already exists on
// disk: it inspects the repo's actual remotes and local git identity/
// signing config and emits a schema-valid TOML snippet reproducing them,
// appended to the target config file.
func runAdd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "path to the git-manager TOML config file (required)")
	groupName := fs.String("group", "", "group the new entry belongs to (required)")
	repoName := fs.String("name", "", "repo name for the new entry (required)")
	dryRun := fs.Bool("dry-run", false, "print the generated entry without writing it")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *configPath == "" {
		fmt.Fprintln(stderr, "git-manager add: -config is required")
		return 2
	}
	if *groupName == "" || *repoName == "" {
		fmt.Fprintln(stderr, "git-manager add: -group and -name are required")
		return 2
	}

	repoPath := "."
	if fs.NArg() > 0 {
		repoPath = fs.Arg(0)
	}
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		fmt.Fprintf(stderr, "git-manager add: %v\n", err)
		return 1
	}
	if _, err := os.Stat(filepath.Join(absPath, ".git")); err != nil {
		fmt.Fprintf(stderr, "git-manager add: %q is not a git repository\n", absPath)
		return 1
	}

	cfg, err := loadOrEmptyConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "git-manager add: %v\n", err)
		return 1
	}
	if cfg.RepoExists(*groupName, *repoName) {
		fmt.Fprintf(stderr, "git-manager add: group %q already declares a repo named %q\n", *groupName, *repoName)
		return 1
	}

	ctx := context.Background()
	c := gitcli.NewClient()

	remotes, err := c.RemoteList(ctx, absPath)
	if err != nil {
		fmt.Fprintf(stderr, "git-manager add: list remotes: %v\n", err)
		return 1
	}
	repoRemotes := make(map[string]config.RemoteConfig, len(remotes))
	for name, url := range remotes {
		repoRemotes[name] = config.RemoteConfig{URL: url}
	}

	group := config.GroupConfig{
		Name: *groupName,
		Path: filepath.Dir(absPath),
		Repos: []config.RepoConfig{
			{
				Name:           *repoName,
				IdentityConfig: readLocalIdentity(ctx, c, absPath),
				Remotes:        repoRemotes,
			},
		},
	}

	if *dryRun {
		fragment, err := config.EncodeGroupFragment(group)
		if err != nil {
			fmt.Fprintf(stderr, "git-manager add: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, fragment)
		return 0
	}

	fragment, err := config.AppendGroupFragment(*configPath, group)
	if err != nil {
		fmt.Fprintf(stderr, "git-manager add: %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, fragment)
	return 0
}

// loadOrEmptyConfig loads path as usual, except a missing file is not an
// error: it's treated as an empty config with no groups, matching
// AppendGroupFragment's own "creates it if missing" behavior for the write
// side of the same case.
func loadOrEmptyConfig(path string) (*config.Config, error) {
	if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
		return &config.Config{}, nil
	}
	return config.Load(path)
}

// readLocalIdentity reads only what's literally present in repo's local
// (never global/system) git config — nothing here is defaulted or
// inferred beyond a direct reverse mapping of what ApplyIdentity (internal/
// sync) itself writes.
func readLocalIdentity(ctx context.Context, c *gitcli.Client, repo string) config.IdentityConfig {
	var id config.IdentityConfig
	if v, err := c.ConfigGet(ctx, repo, "local", "user.name"); err == nil {
		id.UserName = &v
	}
	if v, err := c.ConfigGet(ctx, repo, "local", "user.email"); err == nil {
		id.UserEmail = &v
	}
	if v, err := c.ConfigGet(ctx, repo, "local", "user.signingkey"); err == nil {
		id.SigningKey = &v
	}
	id.SigningMethod = readLocalSigningMethod(ctx, c, repo)
	return id
}

// readLocalSigningMethod reverses the exact mapping ApplyIdentity applies
// (CLAUDE.md's binding signing_method table): commit.gpgsign=false means
// "none"; otherwise gpg.format=openpgp/ssh means "gpg"/"ssh". Anything else
// (unset, or a value this tool never itself writes) leaves it nil.
func readLocalSigningMethod(ctx context.Context, c *gitcli.Client, repo string) *string {
	if v, err := c.ConfigGet(ctx, repo, "local", "commit.gpgsign"); err == nil && v == "false" {
		method := "none"
		return &method
	}
	if v, err := c.ConfigGet(ctx, repo, "local", "gpg.format"); err == nil {
		switch v {
		case "openpgp":
			method := "gpg"
			return &method
		case "ssh":
			method := "ssh"
			return &method
		}
	}
	return nil
}
