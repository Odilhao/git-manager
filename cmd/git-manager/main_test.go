package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Odilhao/git-manager/internal/sync"
)

// TestMain isolates every git invocation these tests trigger from the
// invoking user's real ~/.gitconfig and any system config.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "git-manager-cmd-home")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(home)
	os.Setenv("HOME", home)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Unsetenv("GIT_CONFIG_GLOBAL")
	os.Exit(m.Run())
}

func runFixture(t *testing.T, dir string, args ...string) {
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
		t.Fatalf("fixture setup: git %v: %v\n%s", args, err, out)
	}
}

func fileURL(path string) string {
	return "file://" + filepath.ToSlash(path)
}

// initBareRepo creates a real bare git repo with one commit, for use as a
// file:// clone source.
func initBareRepo(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	runFixture(t, src, "init", "-b", "main")
	runFixture(t, src, "config", "user.name", "Octocat")
	runFixture(t, src, "config", "user.email", "octocat@example.com")
	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	runFixture(t, src, "add", "file.txt")
	runFixture(t, src, "commit", "-m", "initial")

	bare := t.TempDir()
	runFixture(t, "", "clone", "--bare", src, bare)
	return bare
}

func writeConfig(t *testing.T, groupPath, originURL string) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	contents := "[[groups]]\n" +
		"name = \"work\"\n" +
		"path = \"" + filepath.ToSlash(groupPath) + "\"\n\n" +
		"  [[groups.repos]]\n" +
		"  name = \"example-project\"\n\n" +
		"    [groups.repos.remotes.origin]\n" +
		"    url = \"" + originURL + "\"\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func TestRunSync_RequiresConfigFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"sync"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code when -config is missing")
	}
	if stderr.Len() == 0 {
		t.Fatal("expected an error message on stderr when -config is missing")
	}
}

func TestRunSync_MissingConfigFileIsAnError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"sync", "-config", filepath.Join(t.TempDir(), "does-not-exist.toml")}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for a missing config file")
	}
}

func TestRunSync_DryRunJSONReportsWithoutCloning(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	configPath := writeConfig(t, groupPath, fileURL(origin))

	var stdout, stderr bytes.Buffer
	code := run([]string{"sync", "-config", configPath, "--dry-run", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	var report sync.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if !report.DryRun {
		t.Fatalf("report.DryRun = false, want true")
	}
	if len(report.Repos) != 1 || !report.Repos[0].Cloned {
		t.Fatalf("report.Repos = %+v, want one planned clone", report.Repos)
	}
	if _, err := os.Stat(filepath.Join(groupPath, "example-project")); err == nil {
		t.Fatal("--dry-run actually cloned the repo")
	}
}

func TestRunSync_ActuallyClonesAndReturnsZero(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	configPath := writeConfig(t, groupPath, fileURL(origin))

	var stdout, stderr bytes.Buffer
	code := run([]string{"sync", "-config", configPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(groupPath, "example-project", "file.txt")); err != nil {
		t.Fatalf("sync did not actually clone the repo: %v", err)
	}
}

func TestRunSync_OverwriteAndPruneBothRemoveUndeclaredRemotes(t *testing.T) {
	for _, flag := range []string{"--overwrite", "--prune"} {
		t.Run(flag, func(t *testing.T) {
			origin := initBareRepo(t)
			groupPath := t.TempDir()
			repoPath := filepath.Join(groupPath, "example-project")
			if err := os.MkdirAll(repoPath, 0o755); err != nil {
				t.Fatal(err)
			}
			runFixture(t, repoPath, "init", "-b", "main")
			runFixture(t, repoPath, "remote", "add", "origin", fileURL(origin))
			runFixture(t, repoPath, "remote", "add", "scratch", "https://example.com/octocat/scratch.git")
			configPath := writeConfig(t, groupPath, fileURL(origin))

			var stdout, stderr bytes.Buffer
			code := run([]string{"sync", "-config", configPath, flag}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
			}

			out, err := exec.Command("git", "-C", repoPath, "remote").CombinedOutput()
			if err != nil {
				t.Fatalf("git remote: %v\n%s", err, out)
			}
			if bytes.Contains(out, []byte("scratch")) {
				t.Fatalf("%s did not remove undeclared remote 'scratch': %s", flag, out)
			}
		})
	}
}

func TestRunSync_WithoutOverwriteLeavesUndeclaredRemoteAlone(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	repoPath := filepath.Join(groupPath, "example-project")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runFixture(t, repoPath, "init", "-b", "main")
	runFixture(t, repoPath, "remote", "add", "origin", fileURL(origin))
	runFixture(t, repoPath, "remote", "add", "scratch", "https://example.com/octocat/scratch.git")
	configPath := writeConfig(t, groupPath, fileURL(origin))

	var stdout, stderr bytes.Buffer
	code := run([]string{"sync", "-config", configPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	out, err := exec.Command("git", "-C", repoPath, "remote").CombinedOutput()
	if err != nil {
		t.Fatalf("git remote: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("scratch")) {
		t.Fatalf("undeclared remote 'scratch' was removed without --overwrite/--prune: %s", out)
	}
}

func TestRunSync_ReturnsNonZeroOnRepoError(t *testing.T) {
	groupPath := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.toml")
	// A repo with no declared remotes and no existing checkout can't be
	// cloned, which must surface as a per-repo error and a non-zero exit.
	contents := "[[groups]]\n" +
		"name = \"work\"\n" +
		"path = \"" + filepath.ToSlash(groupPath) + "\"\n\n" +
		"  [[groups.repos]]\n" +
		"  name = \"no-origin\"\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"sync", "-config", configPath}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code when a repo fails to sync")
	}
}

func TestRunStatus_RequiresConfigFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"status"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 when -config is missing", code)
	}
	if stderr.Len() == 0 {
		t.Fatal("expected an error message on stderr when -config is missing")
	}
}

func TestRunStatus_MissingConfigFileIsAnError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "-config", filepath.Join(t.TempDir(), "does-not-exist.toml")}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 for a missing config file", code)
	}
}

func TestRunStatus_InSyncRepoReturnsZeroAndDoesNotClone(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	repoPath := filepath.Join(groupPath, "example-project")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runFixture(t, repoPath, "init", "-b", "main")
	runFixture(t, repoPath, "remote", "add", "origin", fileURL(origin))
	configPath := writeConfig(t, groupPath, fileURL(origin))

	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "-config", configPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repoPath, "file.txt")); err == nil {
		t.Fatal("status cloned/fetched into the repo, but it should be read-only")
	}
}

func TestRunStatus_RemoteDriftReturnsOneAndReportsIt(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	repoPath := filepath.Join(groupPath, "example-project")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runFixture(t, repoPath, "init", "-b", "main")
	// origin points somewhere other than the config's declared URL.
	runFixture(t, repoPath, "remote", "add", "origin", "https://example.com/octocat/stale.git")
	configPath := writeConfig(t, groupPath, fileURL(origin))

	before, err := exec.Command("git", "-C", repoPath, "config", "--local", "--list").CombinedOutput()
	if err != nil {
		t.Fatalf("git config --list: %v\n%s", err, before)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "-config", configPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for remote drift; stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("origin")) {
		t.Fatalf("expected the drift report to mention the drifted remote, got: %s", stdout.String())
	}

	after, err := exec.Command("git", "-C", repoPath, "config", "--local", "--list").CombinedOutput()
	if err != nil {
		t.Fatalf("git config --list: %v\n%s", err, after)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("status mutated local git config:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestRunStatus_IdentityDriftReturnsOne(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	repoPath := filepath.Join(groupPath, "example-project")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runFixture(t, repoPath, "init", "-b", "main")
	runFixture(t, repoPath, "remote", "add", "origin", fileURL(origin))

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	contents := "[defaults]\n" +
		"user_email = \"octocat@example.com\"\n\n" +
		"[[groups]]\n" +
		"name = \"work\"\n" +
		"path = \"" + filepath.ToSlash(groupPath) + "\"\n\n" +
		"  [[groups.repos]]\n" +
		"  name = \"example-project\"\n\n" +
		"    [groups.repos.remotes.origin]\n" +
		"    url = \"" + fileURL(origin) + "\"\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "-config", configPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for identity drift; stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}

	out, err := exec.Command("git", "-C", repoPath, "config", "--local", "--get", "user.email").CombinedOutput()
	if err == nil {
		t.Fatalf("status wrote user.email into local git config, want it left unset: %s", out)
	}
}

func TestRunStatus_JSONOutputShape(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	repoPath := filepath.Join(groupPath, "example-project")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runFixture(t, repoPath, "init", "-b", "main")
	runFixture(t, repoPath, "remote", "add", "origin", fileURL(origin))
	configPath := writeConfig(t, groupPath, fileURL(origin))

	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "-config", configPath, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	var decoded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if _, ok := decoded["drifted"].(bool); !ok {
		t.Fatalf("drifted field missing or not a bool: %v", decoded["drifted"])
	}
	repos, ok := decoded["repos"].([]any)
	if !ok || len(repos) != 1 {
		t.Fatalf("repos field missing or not an array of length 1: %v", decoded["repos"])
	}
}

func TestRunUnknownCommandIsAnError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"bogus"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for an unknown command")
	}
}
