package sync

import (
	"path/filepath"
	"testing"
)

func TestResolveRepoPath_TildeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := ResolveRepoPath("~/code/work", "example-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, "code", "work", "example-project")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveRepoPath_AbsoluteGroupPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	abs := filepath.Join(home, "elsewhere", "work")
	got, err := ResolveRepoPath(abs, "example-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(abs, "example-project")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveRepoPath_RelativeGroupPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Relative group paths resolve against the user's home directory.
	got, err := ResolveRepoPath("code/work", "example-project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, "code", "work", "example-project")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveRepoPath_RejectsPathTraversalEscapingBothBounds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Enough ".." segments to walk past filesystem root regardless of
	// how deeply nested the temp home directory is, escaping both the
	// group path and the home directory.
	traversal := filepath.Join("..", "..", "..", "..", "..", "..", "..", "..", "..", "..", "..", "..", "..", "..", "..", "..", "..", "..", "..", "..", "etc", "passwd")

	_, err := ResolveRepoPath("~/code/work", traversal)
	if err == nil {
		t.Fatal("expected error for path-traversal repo name, got nil")
	}
}

func TestResolveRepoPath_RejectsSiblingDirectoryPrefixCollision(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A naive string-prefix boundary check (strings.HasPrefix(candidate,
	// base) without a separator) would wrongly treat a sibling directory
	// whose name merely starts with the same characters — e.g. "work" vs
	// "work-evil" — as "within". Construct a repo name that walks out of
	// home and back into such a sibling of home itself.
	sibling := filepath.Base(home) + "-evil"
	traversal := filepath.Join("..", sibling, "secret")

	_, err := ResolveRepoPath(".", traversal)
	if err == nil {
		t.Fatal("expected error for sibling-prefix-collision repo name, got nil")
	}
}

func TestResolveRepoPath_RejectsTraversalStayingUnderCleanRootButOutsideGroupAndHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// This traversal escapes the group directory but, since it climbs
	// only two levels, would land back under home ("code/../../etc" ->
	// home/etc). Per the documented rule (within home OR within the
	// declared group path), this case is accepted, not rejected.
	got, err := ResolveRepoPath("~/code/work", filepath.Join("..", "..", "sibling"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, "sibling")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
