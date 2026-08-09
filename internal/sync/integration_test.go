package sync_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Odilhao/git-manager/internal/config"
	"github.com/Odilhao/git-manager/internal/gitcli"
	"github.com/Odilhao/git-manager/internal/sync"
)

// TestIntegration_FullSyncFlowEndToEnd exercises the complete sync flow
// against real local bare git repos as file:// fixtures, asserting on
// resulting git state (remotes, branches, local config) rather than just
// exit code.
func TestIntegration_FullSyncFlowEndToEnd(t *testing.T) {
	// Isolate git from the invoking user's real config.
	home, err := os.MkdirTemp("", "integration-test-home")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", home)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Unsetenv("GIT_CONFIG_GLOBAL")

	// Create origin and upstream bare repos with content.
	originBare := initBareRepo(t, "origin-content")
	upstreamBare := initBareRepo(t, "upstream-content")

	// Create an undeclared second bare repo (not in config) to test
	// additive-by-default: it should remain in the checkout after sync.
	undeclaredBare := initBareRepo(t, "undeclared-content")

	// Create the group path where repos will be cloned.
	groupPath := t.TempDir()

	// Manually set up an existing repo with the undeclared remote already
	// present, so we can test that sync doesn't remove it.
	repoPath := filepath.Join(groupPath, "example-project")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "init", "-b", "main")
	runGit(t, repoPath, "remote", "add", "origin", fileURL(originBare))
	runGit(t, repoPath, "remote", "add", "undeclared", fileURL(undeclaredBare))

	// Build a config with a default identity and per-repo signing settings.
	cfg := &config.Config{
		Defaults: config.IdentityConfig{
			UserEmail:     ptrString("octocat@example.com"),
			SigningKey:    ptrString("ABCDEF1234567890"),
			SigningMethod: ptrString("gpg"),
		},
		Groups: []config.GroupConfig{{
			Name:           "work",
			Path:           groupPath,
			IdentityConfig: config.IdentityConfig{}, // Group-level identity defaults; repos override below
			Repos: []config.RepoConfig{{
				Name: "example-project",
				IdentityConfig: config.IdentityConfig{
					UserName: ptrString("Octocat"),
					// Email and signing inherited from defaults.
				},
				Remotes: map[string]config.RemoteConfig{
					"origin": {
						URL:      fileURL(originBare),
						Branches: "main",
					},
					"upstream": {
						URL: fileURL(upstreamBare),
					},
				},
			}},
		}},
	}

	// Run sync against the config.
	ctx := context.Background()
	client := gitcli.NewClient()
	opts := sync.Options{DryRun: false, Overwrite: false}
	report := sync.Run(ctx, client, cfg, opts)

	// Verify the report structure.
	if report.DryRun {
		t.Fatalf("report.DryRun = true, want false")
	}
	if report.Overwrite {
		t.Fatalf("report.Overwrite = true, want false")
	}
	if report.ErrorCount > 0 {
		for _, rr := range report.Repos {
			if rr.Error != "" {
				t.Logf("  %s: %s", rr.Name, rr.Error)
			}
		}
		t.Fatalf("report.ErrorCount = %d, want 0; error details logged above", report.ErrorCount)
	}
	if len(report.Repos) != 1 {
		t.Fatalf("len(report.Repos) = %d, want 1", len(report.Repos))
	}

	rr := report.Repos[0]
	if rr.Name != "example-project" {
		t.Fatalf("repo name = %q, want %q", rr.Name, "example-project")
	}

	// Verify remotes were reconciled: origin was already present so not marked
	// as added, upstream was added, and undeclared (additive by default) is
	// still in the checkout.
	if len(rr.Remotes.Added) != 1 {
		t.Fatalf("len(Remotes.Added) = %d, want 1 (upstream)", len(rr.Remotes.Added))
	}
	if rr.Remotes.Added[0].Name != "upstream" {
		t.Fatalf("added remote name = %q, want %q", rr.Remotes.Added[0].Name, "upstream")
	}

	// Verify undeclared remote still exists in the actual checkout (assertion
	// of additive-by-default, invariant 3).
	remotes := getGitRemotes(t, repoPath)
	if !contains(remotes, "undeclared") {
		t.Fatalf("undeclared remote was removed; present remotes: %v", remotes)
	}
	if !contains(remotes, "origin") {
		t.Fatalf("origin remote missing; present remotes: %v", remotes)
	}
	if !contains(remotes, "upstream") {
		t.Fatalf("upstream remote missing; present remotes: %v", remotes)
	}

	// Verify identity was written to local config only (invariant 8).
	userNameLocal := getGitConfig(t, repoPath, "local", "user.name")
	if userNameLocal != "Octocat" {
		t.Fatalf("user.name in local config = %q, want %q", userNameLocal, "Octocat")
	}

	userEmailLocal := getGitConfig(t, repoPath, "local", "user.email")
	if userEmailLocal != "octocat@example.com" {
		t.Fatalf("user.email in local config = %q, want %q", userEmailLocal, "octocat@example.com")
	}

	gpgFormatLocal := getGitConfig(t, repoPath, "local", "gpg.format")
	if gpgFormatLocal != "openpgp" {
		t.Fatalf("gpg.format in local config = %q, want %q", gpgFormatLocal, "openpgp")
	}

	signingKeyLocal := getGitConfig(t, repoPath, "local", "user.signingkey")
	if signingKeyLocal != "ABCDEF1234567890" {
		t.Fatalf("user.signingkey in local config = %q, want %q", signingKeyLocal, "ABCDEF1234567890")
	}

	// Verify that global config was never touched (part of invariant 8 safety check).
	globalCheck := exec.Command("git", "config", "--global", "--get", "user.name")
	globalCheck.Env = append(os.Environ(), "HOME="+home, "GIT_CONFIG_NOSYSTEM=1")
	_, err = globalCheck.Output()
	if err == nil {
		t.Fatal("global git config was written to, but identity/signing should only touch local scope")
	}
}

// Helper functions for the integration test.

func initBareRepo(t *testing.T, name string) string {
	t.Helper()
	// Create a temporary directory for the working repo.
	src := t.TempDir()
	runGit(t, src, "init", "-b", "main")
	runGit(t, src, "config", "user.name", "Octocat")
	runGit(t, src, "config", "user.email", "octocat@example.com")

	// Create a file and commit it.
	if err := os.WriteFile(filepath.Join(src, "content.txt"), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, src, "add", "content.txt")
	runGit(t, src, "commit", "-m", "initial commit for "+name)

	// Clone it as a bare repo.
	bare := t.TempDir()
	runGit(t, "", "clone", "--bare", src, bare)
	return bare
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	var cmdArgs []string
	if dir != "" {
		cmdArgs = append([]string{"-C", dir}, args...)
	} else {
		cmdArgs = args
	}
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func fileURL(path string) string {
	return "file://" + filepath.ToSlash(path)
}

func getGitRemotes(t *testing.T, repoPath string) []string {
	t.Helper()
	cmd := exec.Command("git", "-C", repoPath, "remote")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git remote: %v", err)
	}
	var remotes []string
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		if len(line) > 0 {
			remotes = append(remotes, string(line))
		}
	}
	return remotes
}

func getGitConfig(t *testing.T, repoPath, scope, key string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repoPath, "config", "--"+scope, "--get", key)
	out, err := cmd.Output()
	if err != nil {
		// Return empty string if the key doesn't exist (exit code 1).
		// Any other error is a real problem.
		if cmd.ProcessState.ExitCode() == 1 {
			return ""
		}
		t.Fatalf("git config --%s --get %s: %v", scope, key, err)
	}
	return string(bytes.TrimSpace(out))
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func ptrString(s string) *string {
	return &s
}
