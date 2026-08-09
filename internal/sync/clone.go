package sync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// gitClient is the subset of *gitcli.Client CloneIfMissing depends on.
type gitClient interface {
	Clone(ctx context.Context, url, path string) error
}

// NotAGitRepoError reports that path exists but is not a valid git
// repository, so cloning into or over it was refused.
type NotAGitRepoError struct {
	Path string
}

func (e *NotAGitRepoError) Error() string {
	return fmt.Sprintf("sync: %q exists but is not a valid git repository", e.Path)
}

// CloneIfMissing ensures a local git checkout exists at path. If path
// doesn't exist, it clones originURL into it via gitcli. If path exists and
// is already a valid git repository, it's left untouched. If path exists but
// isn't a valid git repository, it returns a *NotAGitRepoError rather than
// cloning into or over it.
func CloneIfMissing(ctx context.Context, c gitClient, path, originURL string) error {
	_, statErr := os.Stat(path)
	switch {
	case errors.Is(statErr, os.ErrNotExist):
		if err := c.Clone(ctx, originURL, path); err != nil {
			return fmt.Errorf("sync: clone %q into %q: %w", originURL, path, err)
		}
		return nil
	case statErr != nil:
		return fmt.Errorf("sync: stat %q: %w", path, statErr)
	}

	// A ".git" entry (directory for a normal checkout, file for a
	// worktree) is what makes path a git repository at all — checking for
	// it, rather than resolving a ref, also correctly accepts a freshly
	// initialized repo that has no commits yet.
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		return &NotAGitRepoError{Path: path}
	}
	return nil
}
