// Package sync resolves declared config state into local filesystem
// operations.
package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveRepoPath combines a group's declared path with a repo name into
// the canonical local checkout path.
//
// groupPath supports "~" expansion to the user's home directory. A relative
// groupPath (no leading "~" or "/") resolves against the home directory too,
// so "code/work" and "~/code/work" are equivalent — there is no notion of
// "relative to the config file" here, only relative to home.
//
// The resolved, cleaned, absolute path must fall within the user's home
// directory or within the resolved group path itself; otherwise repoName is
// rejected as a path-traversal attempt.
func ResolveRepoPath(groupPath, repoName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve repo path: determine home directory: %w", err)
	}

	absGroup := expandToAbs(groupPath, home)

	candidate := filepath.Clean(filepath.Join(absGroup, repoName))

	if !isWithin(candidate, home) && !isWithin(candidate, absGroup) {
		return "", fmt.Errorf("resolve repo path: repo name %q escapes both home directory %q and group path %q", repoName, home, groupPath)
	}

	return candidate, nil
}

// expandToAbs expands a leading "~" and resolves relative paths against home.
func expandToAbs(p, home string) string {
	switch {
	case p == "~":
		p = home
	case strings.HasPrefix(p, "~/"):
		p = filepath.Join(home, p[len("~/"):])
	case !filepath.IsAbs(p):
		p = filepath.Join(home, p)
	}
	return filepath.Clean(p)
}

// isWithin reports whether candidate (already cleaned and absolute) is base
// itself or a descendant of it.
func isWithin(candidate, base string) bool {
	base = filepath.Clean(base)
	if candidate == base {
		return true
	}
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
