package gitcli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain isolates every git invocation in this package's tests — both the
// fixture setup below and the Client under test — from the invoking user's
// real ~/.gitconfig and any system config.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "gitcli-home")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(home)
	os.Setenv("HOME", home)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Unsetenv("GIT_CONFIG_GLOBAL")
	os.Exit(m.Run())
}

// initRepo creates a real local git repo with one commit and returns its path.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runFixture(t, dir, "init", "-b", "main")
	runFixture(t, dir, "config", "user.name", "Octocat")
	runFixture(t, dir, "config", "user.email", "octocat@example.com")
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	runFixture(t, dir, "add", "file.txt")
	runFixture(t, dir, "commit", "-m", "initial")
	return dir
}

func runFixture(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture setup: git %v: %v\n%s", args, err, out)
	}
}

func fileURL(path string) string {
	return "file://" + filepath.ToSlash(path)
}

func TestCloneCreatesWorkingCheckout(t *testing.T) {
	src := initRepo(t)
	dst := filepath.Join(t.TempDir(), "clone")
	c := NewClient()

	if err := c.Clone(context.Background(), fileURL(src), dst); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "file.txt")); err != nil {
		t.Fatalf("cloned checkout missing expected file: %v", err)
	}
}

func TestCloneSurfacesGitError(t *testing.T) {
	c := NewClient()
	err := c.Clone(context.Background(), fileURL(filepath.Join(t.TempDir(), "does-not-exist")), filepath.Join(t.TempDir(), "dst"))
	if err == nil {
		t.Fatal("expected error cloning a nonexistent source, got nil")
	}
	if !strings.Contains(err.Error(), "does not appear to be a git repository") && !strings.Contains(err.Error(), "No such file or directory") {
		t.Fatalf("error does not surface git's stderr, got: %v", err)
	}
}

func TestRemoteAddSetURLRemove(t *testing.T) {
	repo := initRepo(t)
	c := NewClient()
	ctx := context.Background()

	if err := c.RemoteAdd(ctx, repo, "origin", "https://example.com/octocat/example.git"); err != nil {
		t.Fatalf("RemoteAdd: %v", err)
	}
	got, err := c.ConfigGet(ctx, repo, "local", "remote.origin.url")
	if err != nil {
		t.Fatalf("ConfigGet after RemoteAdd: %v", err)
	}
	if got != "https://example.com/octocat/example.git" {
		t.Fatalf("remote.origin.url = %q, want the added URL", got)
	}

	if err := c.RemoteSetURL(ctx, repo, "origin", "https://example.com/octocat/renamed.git"); err != nil {
		t.Fatalf("RemoteSetURL: %v", err)
	}
	got, err = c.ConfigGet(ctx, repo, "local", "remote.origin.url")
	if err != nil {
		t.Fatalf("ConfigGet after RemoteSetURL: %v", err)
	}
	if got != "https://example.com/octocat/renamed.git" {
		t.Fatalf("remote.origin.url = %q, want the updated URL", got)
	}

	if err := c.RemoteRemove(ctx, repo, "origin"); err != nil {
		t.Fatalf("RemoteRemove: %v", err)
	}
	if _, err := c.ConfigGet(ctx, repo, "local", "remote.origin.url"); err == nil {
		t.Fatal("expected ConfigGet to fail after RemoteRemove, got nil error")
	}
}

func TestConfigSetAndGet(t *testing.T) {
	repo := initRepo(t)
	c := NewClient()
	ctx := context.Background()

	if err := c.ConfigSet(ctx, repo, "local", "user.email", "octocat@work.example.com"); err != nil {
		t.Fatalf("ConfigSet: %v", err)
	}
	got, err := c.ConfigGet(ctx, repo, "local", "user.email")
	if err != nil {
		t.Fatalf("ConfigGet: %v", err)
	}
	if got != "octocat@work.example.com" {
		t.Fatalf("user.email = %q, want the set value", got)
	}
}

func TestConfigRejectsInvalidScope(t *testing.T) {
	repo := initRepo(t)
	c := NewClient()
	ctx := context.Background()

	if _, err := c.ConfigGet(ctx, repo, "bogus", "user.email"); err == nil || !strings.Contains(err.Error(), "invalid scope") {
		t.Fatalf("ConfigGet: expected our own invalid-scope error, got: %v", err)
	}
	if err := c.ConfigSet(ctx, repo, "bogus", "user.email", "x@example.com"); err == nil || !strings.Contains(err.Error(), "invalid scope") {
		t.Fatalf("ConfigSet: expected our own invalid-scope error, got: %v", err)
	}
}

// TestConfigGlobalScopeRoutesToGlobalConfig proves "global" scope is passed
// through to git as --global rather than falling back to --local: it points
// GIT_CONFIG_GLOBAL at a private file and checks the value lands there, not
// in the repo's own .git/config.
func TestConfigGlobalScopeRoutesToGlobalConfig(t *testing.T) {
	repo := initRepo(t)
	globalConfig := filepath.Join(t.TempDir(), "gitconfig-global")
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	c := NewClient()
	ctx := context.Background()

	if err := c.ConfigSet(ctx, repo, "global", "user.email", "octocat@global.example.com"); err != nil {
		t.Fatalf("ConfigSet(global): %v", err)
	}

	got, err := c.ConfigGet(ctx, repo, "global", "user.email")
	if err != nil {
		t.Fatalf("ConfigGet(global): %v", err)
	}
	if got != "octocat@global.example.com" {
		t.Fatalf("ConfigGet(global) = %q, want the value set at global scope", got)
	}

	data, err := os.ReadFile(globalConfig)
	if err != nil {
		t.Fatalf("reading global config file: %v", err)
	}
	if !strings.Contains(string(data), "octocat@global.example.com") {
		t.Fatalf("global config file %s does not contain the set value: %s", globalConfig, data)
	}

	if localVal, err := c.ConfigGet(ctx, repo, "local", "user.email"); err == nil && localVal == "octocat@global.example.com" {
		t.Fatalf("value set at global scope leaked into the repo's local config: %q", localVal)
	}
}

// TestConfigSystemScopeRoutesToSystemConfig mirrors the global-scope test
// for "system": GIT_CONFIG_SYSTEM (git >= 2.32) redirects --system to a
// private file instead of the real /etc/gitconfig.
func TestConfigSystemScopeRoutesToSystemConfig(t *testing.T) {
	repo := initRepo(t)
	systemConfig := filepath.Join(t.TempDir(), "gitconfig-system")
	t.Setenv("GIT_CONFIG_SYSTEM", systemConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "0")
	c := NewClient()
	ctx := context.Background()

	if err := c.ConfigSet(ctx, repo, "system", "user.email", "octocat@system.example.com"); err != nil {
		t.Fatalf("ConfigSet(system): %v", err)
	}

	got, err := c.ConfigGet(ctx, repo, "system", "user.email")
	if err != nil {
		t.Fatalf("ConfigGet(system): %v", err)
	}
	if got != "octocat@system.example.com" {
		t.Fatalf("ConfigGet(system) = %q, want the value set at system scope", got)
	}

	data, err := os.ReadFile(systemConfig)
	if err != nil {
		t.Fatalf("reading system config file: %v", err)
	}
	if !strings.Contains(string(data), "octocat@system.example.com") {
		t.Fatalf("system config file %s does not contain the set value: %s", systemConfig, data)
	}

	if localVal, err := c.ConfigGet(ctx, repo, "local", "user.email"); err == nil && localVal == "octocat@system.example.com" {
		t.Fatalf("value set at system scope leaked into the repo's local config: %q", localVal)
	}
}

func TestFetchWithRefSpec(t *testing.T) {
	upstream := initRepo(t)
	downstream := t.TempDir()
	c := NewClient()
	ctx := context.Background()

	if err := c.Clone(ctx, fileURL(upstream), downstream); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	runFixture(t, upstream, "commit", "--allow-empty", "-m", "second")

	// This refspec targets a ref the clone's default fetch refspec never
	// touches, so RevParse below only succeeds if Fetch actually passed it
	// through to git rather than ignoring it.
	if err := c.Fetch(ctx, downstream, "origin", "+refs/heads/main:refs/custom/pinned"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	got, err := c.RevParse(ctx, downstream, "refs/custom/pinned")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	want, err := c.RevParse(ctx, upstream, "HEAD")
	if err != nil {
		t.Fatalf("RevParse upstream: %v", err)
	}
	if got != want {
		t.Fatalf("fetched ref = %q, want %q (upstream HEAD after custom refspec fetch)", got, want)
	}
}

func TestLSRemote(t *testing.T) {
	repo := initRepo(t)
	c := NewClient()

	out, err := c.LSRemote(context.Background(), fileURL(repo))
	if err != nil {
		t.Fatalf("LSRemote: %v", err)
	}
	if !strings.Contains(out, "refs/heads/main") {
		t.Fatalf("LSRemote output missing refs/heads/main, got: %q", out)
	}
}

func TestRevParse(t *testing.T) {
	repo := initRepo(t)
	c := NewClient()

	got, err := c.RevParse(context.Background(), repo, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	if len(got) != 40 {
		t.Fatalf("RevParse HEAD = %q, want a 40-char commit hash", got)
	}
}

func TestRevParseSurfacesGitStderr(t *testing.T) {
	repo := initRepo(t)
	c := NewClient()

	_, err := c.RevParse(context.Background(), repo, "not-a-real-ref")
	if err == nil {
		t.Fatal("expected error for unresolvable ref, got nil")
	}
	if !strings.Contains(err.Error(), "not-a-real-ref") {
		t.Fatalf("error does not surface git's stderr about the bad ref, got: %v", err)
	}
}

func TestContextCancellationIsHonored(t *testing.T) {
	repo := initRepo(t)
	c := NewClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.RevParse(ctx, repo, "HEAD")
	if err == nil {
		t.Fatal("expected error for an already-cancelled context, got nil")
	}
}

func TestContextTimeoutIsHonored(t *testing.T) {
	repo := initRepo(t)
	c := NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	_, err := c.RevParse(ctx, repo, "HEAD")
	if err == nil {
		t.Fatal("expected error for a timed-out context, got nil")
	}
}
