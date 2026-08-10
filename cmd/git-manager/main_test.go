package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Odilhao/git-manager/internal/status"
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

	// duration_ms is real elapsed time; a fast dry-run can legitimately
	// round to 0ms, so only its presence (never negative) is checked here.
	// TestRunSync_ActuallyClonesAndReturnsZero, which does a real clone,
	// checks the un-normalized value is > 0.
	if report.DurationMS < 0 {
		t.Fatalf("report.DurationMS = %d, want >= 0", report.DurationMS)
	}
	if report.Repos[0].DurationMS < 0 {
		t.Fatalf("report.Repos[0].DurationMS = %d, want >= 0", report.Repos[0].DurationMS)
	}
	if report.Repos[0].Outcome != "success" {
		t.Fatalf("report.Repos[0].Outcome = %q, want %q", report.Repos[0].Outcome, "success")
	}

	// Normalize the nondeterministic duration before comparing against the
	// golden file, whose value is always 0.
	report.DurationMS = 0
	report.Repos[0].DurationMS = 0

	// Load and compare against the golden file to verify the JSON structure
	// matches the expected format.
	goldenData, err := os.ReadFile("testdata/sync_dryrun.golden")
	if err != nil {
		t.Fatalf("failed to load golden file: %v", err)
	}

	var goldenReport sync.Report
	if err := json.Unmarshal(goldenData, &goldenReport); err != nil {
		t.Fatalf("golden file is not valid JSON: %v", err)
	}

	// Compare structure: both should have same number of repos, same dry_run/overwrite/error_count values
	if report.DryRun != goldenReport.DryRun {
		t.Fatalf("report.DryRun = %v, want %v", report.DryRun, goldenReport.DryRun)
	}
	if report.Overwrite != goldenReport.Overwrite {
		t.Fatalf("report.Overwrite = %v, want %v", report.Overwrite, goldenReport.Overwrite)
	}
	if report.DurationMS != goldenReport.DurationMS {
		t.Fatalf("normalized report.DurationMS = %v, want %v", report.DurationMS, goldenReport.DurationMS)
	}
	if len(report.Repos) != len(goldenReport.Repos) {
		t.Fatalf("len(report.Repos) = %d, want %d", len(report.Repos), len(goldenReport.Repos))
	}

	// Verify the repo structure matches
	rr := report.Repos[0]
	grr := goldenReport.Repos[0]
	if rr.Name != grr.Name {
		t.Fatalf("repo.Name = %q, want %q", rr.Name, grr.Name)
	}
	if rr.Cloned != grr.Cloned {
		t.Fatalf("repo.Cloned = %v, want %v", rr.Cloned, grr.Cloned)
	}
	if len(rr.Remotes.Added) != len(grr.Remotes.Added) {
		t.Fatalf("len(Remotes.Added) = %d, want %d", len(rr.Remotes.Added), len(grr.Remotes.Added))
	}
	if len(rr.Fetches) != len(grr.Fetches) {
		t.Fatalf("len(Fetches) = %d, want %d", len(rr.Fetches), len(grr.Fetches))
	}
	if rr.Outcome != grr.Outcome {
		t.Fatalf("repo.Outcome = %q, want %q", rr.Outcome, grr.Outcome)
	}
	if rr.DurationMS != grr.DurationMS {
		t.Fatalf("normalized repo.DurationMS = %v, want %v", rr.DurationMS, grr.DurationMS)
	}
}

func TestRunSync_ActuallyClonesAndReturnsZero(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	configPath := writeConfig(t, groupPath, fileURL(origin))

	var stdout, stderr bytes.Buffer
	code := run([]string{"sync", "-config", configPath, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(groupPath, "example-project", "file.txt")); err != nil {
		t.Fatalf("sync did not actually clone the repo: %v", err)
	}

	var report sync.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	// A real clone over file:// is slow enough relative to a millisecond
	// clock tick that duration_ms must be strictly positive here, unlike
	// the fast dry-run planning path.
	if report.DurationMS <= 0 {
		t.Fatalf("report.DurationMS = %d, want > 0 for a real clone", report.DurationMS)
	}
	if len(report.Repos) != 1 || report.Repos[0].DurationMS <= 0 {
		t.Fatalf("report.Repos[0].DurationMS = %+v, want > 0 for a real clone", report.Repos)
	}
	if report.Repos[0].Outcome != "success" {
		t.Fatalf("report.Repos[0].Outcome = %q, want %q", report.Repos[0].Outcome, "success")
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

// TestRunSync_JSONOutcomeFailureGolden exercises the "failure" Outcome value
// (an error with no activity recorded) and checks it against a golden file,
// with duration_ms normalized to 0.
func TestRunSync_JSONOutcomeFailureGolden(t *testing.T) {
	groupPath := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.toml")
	contents := "[[groups]]\n" +
		"name = \"work\"\n" +
		"path = \"" + filepath.ToSlash(groupPath) + "\"\n\n" +
		"  [[groups.repos]]\n" +
		"  name = \"no-origin\"\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"sync", "-config", configPath, "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code when a repo fails to sync")
	}

	var report sync.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if len(report.Repos) != 1 {
		t.Fatalf("len(report.Repos) = %d, want 1", len(report.Repos))
	}
	if report.Repos[0].Outcome != "failure" {
		t.Fatalf("report.Repos[0].Outcome = %q, want %q", report.Repos[0].Outcome, "failure")
	}
	if report.Repos[0].Error == "" {
		t.Fatal("report.Repos[0].Error is empty, want the no-origin error")
	}
	if report.Repos[0].DurationMS < 0 {
		t.Fatalf("report.Repos[0].DurationMS = %d, want >= 0", report.Repos[0].DurationMS)
	}

	report.Repos[0].DurationMS = 0
	report.Repos[0].Path = "<placeholder-path>"
	report.Repos[0].Error = "<placeholder-error>"
	report.DurationMS = 0
	assertGolden(t, "testdata/sync_failure.golden", report)
}

// TestRunSync_JSONOutcomePartialGolden exercises the "partial" Outcome value
// (an error recorded after some activity — here, a remote URL update — was
// already applied) and checks it against a golden file, with duration_ms
// normalized to 0.
func TestRunSync_JSONOutcomePartialGolden(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	repoPath := filepath.Join(groupPath, "example-project")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runFixture(t, repoPath, "init", "-b", "main")
	runFixture(t, repoPath, "config", "user.name", "Octocat")
	runFixture(t, repoPath, "config", "user.email", "octocat@example.com")
	runFixture(t, repoPath, "remote", "add", "origin", fileURL(origin))
	// Declaring a different, unreachable origin URL forces a recorded
	// remote update before the subsequent fetch against it fails.
	configPath := writeConfig(t, groupPath, "https://127.0.0.1:1/does-not-exist.git")

	var stdout, stderr bytes.Buffer
	code := run([]string{"sync", "-config", configPath, "--json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for a fetch failure against an unreachable remote")
	}

	var report sync.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if len(report.Repos) != 1 {
		t.Fatalf("len(report.Repos) = %d, want 1", len(report.Repos))
	}
	if report.Repos[0].Outcome != "partial" {
		t.Fatalf("report.Repos[0].Outcome = %q, want %q", report.Repos[0].Outcome, "partial")
	}
	if len(report.Repos[0].Remotes.Updated) == 0 {
		t.Fatalf("report.Repos[0].Remotes.Updated = %+v, want the origin URL update recorded", report.Repos[0].Remotes.Updated)
	}

	report.Repos[0].DurationMS = 0
	report.Repos[0].Path = "<placeholder-path>"
	report.Repos[0].Error = "<placeholder-error>"
	report.DurationMS = 0
	assertGolden(t, "testdata/sync_partial.golden", report)
}

// assertGolden compares v, marshaled with the same indentation the sync/
// status --json output uses, against the contents of goldenPath.
func assertGolden(t *testing.T, goldenPath string, v any) {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to load golden file %s: %v", goldenPath, err)
	}
	// git checks this repo out with core.autocrlf=true, so a golden file on
	// disk may carry \r\n while json.Encoder always writes \n; strip \r from
	// both sides so the comparison is about content, not the checkout's line
	// endings (the same reason the pre-existing sync_dryrun.golden test
	// compares unmarshaled fields rather than raw bytes).
	got := strings.ReplaceAll(buf.String(), "\r\n", "\n")
	wantNormalized := strings.ReplaceAll(string(want), "\r\n", "\n")
	if got != wantNormalized {
		t.Fatalf("output does not match %s:\ngot:\n%s\nwant:\n%s", goldenPath, got, wantNormalized)
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

// TestRunStatus_JSONOutcomeGolden exercises status's mirrored Outcome and
// DurationMS fields against a golden file, using a repo with remote drift so
// Remotes.Updated is populated too.
func TestRunStatus_JSONOutcomeGolden(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	repoPath := filepath.Join(groupPath, "example-project")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runFixture(t, repoPath, "init", "-b", "main")
	runFixture(t, repoPath, "remote", "add", "origin", "https://example.com/octocat/stale.git")
	configPath := writeConfig(t, groupPath, fileURL(origin))

	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "-config", configPath, "--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for remote drift; stderr: %s", code, stderr.String())
	}

	var report status.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if len(report.Repos) != 1 {
		t.Fatalf("len(report.Repos) = %d, want 1", len(report.Repos))
	}
	if report.Repos[0].Outcome != "success" {
		t.Fatalf("report.Repos[0].Outcome = %q, want %q (drift with no sync error)", report.Repos[0].Outcome, "success")
	}
	if len(report.Repos[0].Remotes.Updated) == 0 {
		t.Fatalf("report.Repos[0].Remotes.Updated = %+v, want the stale origin URL recorded", report.Repos[0].Remotes.Updated)
	}
	if report.DurationMS < 0 || report.Repos[0].DurationMS < 0 {
		t.Fatalf("negative duration_ms: report=%d repo=%d", report.DurationMS, report.Repos[0].DurationMS)
	}

	report.Repos[0].DurationMS = 0
	report.Repos[0].Path = "<placeholder-path>"
	for i := range report.Repos[0].Remotes.Updated {
		report.Repos[0].Remotes.Updated[i].URL = "<placeholder-repo-url>"
	}
	report.DurationMS = 0
	assertGolden(t, "testdata/status_json.golden", report)
}

// TestRunStatus_JSONOutcomeFailureGolden exercises status's mirrored
// Outcome for the "failure" value (a repo status can't even plan for,
// because it has no declared origin and no local checkout), proving the
// mirror isn't hardcoded to "success" — the only value the other status
// golden test exercises.
func TestRunStatus_JSONOutcomeFailureGolden(t *testing.T) {
	groupPath := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "config.toml")
	contents := "[[groups]]\n" +
		"name = \"work\"\n" +
		"path = \"" + filepath.ToSlash(groupPath) + "\"\n\n" +
		"  [[groups.repos]]\n" +
		"  name = \"no-origin\"\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "-config", configPath, "--json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for an errored repo; stderr: %s", code, stderr.String())
	}

	var report status.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if len(report.Repos) != 1 {
		t.Fatalf("len(report.Repos) = %d, want 1", len(report.Repos))
	}
	if report.Repos[0].Outcome != "failure" {
		t.Fatalf("report.Repos[0].Outcome = %q, want %q", report.Repos[0].Outcome, "failure")
	}
	if report.Repos[0].Error == "" {
		t.Fatal("report.Repos[0].Error is empty, want the no-origin error")
	}

	report.Repos[0].DurationMS = 0
	report.Repos[0].Path = "<placeholder-path>"
	report.Repos[0].Error = "<placeholder-error>"
	report.DurationMS = 0
	assertGolden(t, "testdata/status_json_failure.golden", report)
}

func TestRunUnknownCommandIsAnError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"bogus"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for an unknown command")
	}
}

// TestRun_NoArgsPrintsUsageAndReturnsZero covers the no-args top-level case,
// which today prints a one-line usage message to stderr and exits 1; issue
// #51 requires it to behave like top-level help instead (stdout, exit 0,
// every subcommand named).
func TestRun_NoArgsPrintsUsageAndReturnsZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(nil) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("run(nil) wrote to stderr, want stdout only: %s", stderr.String())
	}
	for _, name := range []string{"sync", "status", "add", "install", "uninstall"} {
		if !strings.Contains(stdout.String(), name) {
			t.Fatalf("top-level usage missing command %q:\n%s", name, stdout.String())
		}
	}
}

// TestRun_TopLevelHelpFlagsPrintUsageAndReturnZero checks that help, -h and
// --help all produce the identical top-level summary as the no-args case.
func TestRun_TopLevelHelpFlagsPrintUsageAndReturnZero(t *testing.T) {
	for _, flag := range []string{"help", "-h", "--help"} {
		t.Run(flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{flag}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("run([%q]) = %d, want 0; stderr: %s", flag, code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("run([%q]) wrote to stderr, want stdout only: %s", flag, stderr.String())
			}
			if !strings.Contains(stdout.String(), "sync") {
				t.Fatalf("run([%q]) usage missing %q:\n%s", flag, "sync", stdout.String())
			}
		})
	}
}

// TestRunSync_HelpFlagsPrintFlagUsageAndReturnZero checks -h/--help/help are
// handled before fs.Parse: they must never collide with flag.ErrHelp's usual
// exit-2 path, and they must not require -config.
func TestRunSync_HelpFlagsPrintFlagUsageAndReturnZero(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "help"} {
		t.Run(flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{"sync", flag}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("run([sync %q]) = %d, want 0; stderr: %s", flag, code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("run([sync %q]) wrote to stderr, want stdout only: %s", flag, stderr.String())
			}
			if !strings.Contains(stdout.String(), "-config") {
				t.Fatalf("run([sync %q]) usage missing flag -config:\n%s", flag, stdout.String())
			}
		})
	}
}

func TestRunStatus_HelpFlagsPrintFlagUsageAndReturnZero(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "help"} {
		t.Run(flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{"status", flag}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("run([status %q]) = %d, want 0; stderr: %s", flag, code, stderr.String())
			}
			if !strings.Contains(stdout.String(), "-config") {
				t.Fatalf("run([status %q]) usage missing flag -config:\n%s", flag, stdout.String())
			}
		})
	}
}

func TestRunInstall_HelpFlagsPrintFlagUsageAndReturnZero(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "help"} {
		t.Run(flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{"install", flag}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("run([install %q]) = %d, want 0; stderr: %s", flag, code, stderr.String())
			}
			if !strings.Contains(stdout.String(), "-dry-run") {
				t.Fatalf("run([install %q]) usage missing flag -dry-run:\n%s", flag, stdout.String())
			}
		})
	}
}

func TestRunUninstall_HelpFlagsPrintFlagUsageAndReturnZero(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "help"} {
		t.Run(flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{"uninstall", flag}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("run([uninstall %q]) = %d, want 0; stderr: %s", flag, code, stderr.String())
			}
			if !strings.Contains(stdout.String(), "-dry-run") {
				t.Fatalf("run([uninstall %q]) usage missing flag -dry-run:\n%s", flag, stdout.String())
			}
		})
	}
}

// TestRunSync_UnknownFlagStillErrorsWithExitCode2 pins the pre-existing
// real-error behavior: an actual unknown flag must keep going through
// fs.Parse and returning 2, unaffected by the new help-detection branch.
func TestRunSync_UnknownFlagStillErrorsWithExitCode2(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"sync", "-bogus-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run([sync -bogus-flag]) = %d, want 2; stdout: %s stderr: %s", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("run([sync -bogus-flag]) wrote to stdout, want stderr only: %s", stdout.String())
	}
}

func TestRunInstall_DryRunOutputsActionsAndReturnsZero(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	var stdout, stderr bytes.Buffer
	code := run([]string{"install", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("expected stdout output for install --dry-run")
	}
	output := stdout.String()
	if stdout.Len() == 0 {
		t.Fatal("expected non-empty stdout")
	}
	// The output should contain some action descriptions. On the test platform,
	// it will be a "would write" or similar message, or a "not yet supported" message
	// on Windows. Just verify there's some content.
	if len(output) == 0 {
		t.Fatal("expected action output but got empty string")
	}
}

func TestRunUninstall_DryRunOutputsActionsAndReturnsZero(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	var stdout, stderr bytes.Buffer
	code := run([]string{"uninstall", "--dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("expected stdout output for uninstall --dry-run")
	}
	output := stdout.String()
	// The output should contain some action descriptions.
	if len(output) == 0 {
		t.Fatal("expected action output but got empty string")
	}
}
