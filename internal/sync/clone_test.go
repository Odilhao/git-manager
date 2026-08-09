package sync

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Odilhao/git-manager/internal/gitcli"
)

// TestMain isolates every git invocation in this package's tests from the
// invoking user's real ~/.gitconfig and any system config.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "sync-home")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(home)
	os.Setenv("HOME", home)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Unsetenv("GIT_CONFIG_GLOBAL")
	os.Exit(m.Run())
}

// initBareRepo creates a real bare git repo with one commit on its default
// branch and returns its path, for use as a file:// clone source.
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

func TestCloneIfMissing_ClonesIntoMissingPath(t *testing.T) {
	origin := initBareRepo(t)
	dst := filepath.Join(t.TempDir(), "checkout")
	c := gitcli.NewClient()

	if err := CloneIfMissing(context.Background(), c, dst, fileURL(origin)); err != nil {
		t.Fatalf("CloneIfMissing: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "file.txt")); err != nil {
		t.Fatalf("cloned checkout missing expected file: %v", err)
	}
}

func TestCloneIfMissing_LeavesExistingValidRepoAlone(t *testing.T) {
	origin := initBareRepo(t)
	dst := t.TempDir()
	runFixture(t, dst, "init", "-b", "main")
	runFixture(t, dst, "remote", "add", "origin", "https://example.invalid/never-cloned.git")
	c := gitcli.NewClient()

	if err := CloneIfMissing(context.Background(), c, dst, fileURL(origin)); err != nil {
		t.Fatalf("CloneIfMissing: %v", err)
	}

	// A real clone would have set origin to fileURL(origin) and populated
	// file.txt; observing the untouched placeholder remote and the absence
	// of the source file proves no clone was attempted.
	got, err := c.ConfigGet(context.Background(), dst, "local", "remote.origin.url")
	if err != nil {
		t.Fatalf("ConfigGet: %v", err)
	}
	if got != "https://example.invalid/never-cloned.git" {
		t.Fatalf("remote.origin.url changed to %q; repo was re-cloned over", got)
	}
	if _, err := os.Stat(filepath.Join(dst, "file.txt")); err == nil {
		t.Fatal("file.txt present; existing repo was re-cloned over")
	}
}

func TestCloneIfMissing_ExistingNonRepoPathIsAnError(t *testing.T) {
	origin := initBareRepo(t)
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "not-a-repo"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := gitcli.NewClient()

	err := CloneIfMissing(context.Background(), c, dst, fileURL(origin))
	if err == nil {
		t.Fatal("expected error for existing non-repo path, got nil")
	}
	var notRepoErr *NotAGitRepoError
	if !errors.As(err, &notRepoErr) {
		t.Fatalf("expected *NotAGitRepoError, got %T: %v", err, err)
	}
	if _, statErr := os.Stat(filepath.Join(dst, "file.txt")); statErr == nil {
		t.Fatal("file.txt present; clone was attempted over a non-repo path")
	}
}

func TestCloneIfMissing_CloneFailureIsDistinguishable(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "checkout")
	c := gitcli.NewClient()

	err := CloneIfMissing(context.Background(), c, dst, fileURL(filepath.Join(t.TempDir(), "does-not-exist")))
	if err == nil {
		t.Fatal("expected error for a failing clone, got nil")
	}
	var gitErr *gitcli.Error
	if !errors.As(err, &gitErr) {
		t.Fatalf("expected clone failure to wrap *gitcli.Error, got %T: %v", err, err)
	}
	var notRepoErr *NotAGitRepoError
	if errors.As(err, &notRepoErr) {
		t.Fatal("clone failure must not be reported as NotAGitRepoError")
	}
}

func TestCloneIfMissing_GenericStatErrorIsNotMisclassified(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks are bypassed for root")
	}
	origin := initBareRepo(t)
	parent := t.TempDir()
	dst := filepath.Join(parent, "checkout")
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(parent, 0o755) })
	// Denying execute on the parent makes os.Stat(dst) fail with a
	// permission error rather than os.ErrNotExist, without dst itself
	// looking like anything other than an ordinary directory.
	if err := os.Chmod(parent, 0o000); err != nil {
		t.Fatal(err)
	}
	c := gitcli.NewClient()

	err := CloneIfMissing(context.Background(), c, dst, fileURL(origin))
	if err == nil {
		t.Fatal("expected error for an unreadable path, got nil")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected a wrapped permission error, got %T: %v", err, err)
	}
	var notRepoErr *NotAGitRepoError
	if errors.As(err, &notRepoErr) {
		t.Fatal("a generic stat failure must not be reported as NotAGitRepoError")
	}
}
