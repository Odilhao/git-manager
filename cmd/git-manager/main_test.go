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

func TestRunUnknownCommandIsAnError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"bogus"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for an unknown command")
	}
}
