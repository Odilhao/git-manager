package sync

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// fetchClient is the subset of *gitcli.Client FetchBranches depends on.
type fetchClient interface {
	Fetch(ctx context.Context, repo, remote string, refSpecs ...string) error
	LSRemote(ctx context.Context, url string) (string, error)
}

// FetchReport records what FetchBranches actually asked git to fetch.
type FetchReport struct {
	// Mode is "all", "glob" or "regex" — which selection path ran.
	Mode string
	// RefSpecs is exactly what was passed to `git fetch`; empty for Mode "all".
	RefSpecs []string
	// Branches lists the branch names matched, populated only for Mode
	// "regex". A glob refspec's matches are resolved by git itself
	// server-side, so which branches it matched isn't known here without an
	// ls-remote call — and glob mode deliberately never makes one.
	Branches []string
}

// regexOnlyChars are metacharacters meaningful in a regex that can never
// appear in a literal git ref name component: "^" is explicitly forbidden by
// git-check-ref-format, and "$", "|", "(", ")", "+", "\" are never used by
// this project's glob patterns (whose own wildcards are "*" and "?" only).
// Their presence is the signal that a `branches` value is a regex rather
// than a glob, so this check is total and unambiguous — every pattern maps
// to exactly one mode, never both.
const regexOnlyChars = `^$|()+\`

func isRegexPattern(pattern string) bool {
	return strings.ContainsAny(pattern, regexOnlyChars)
}

// FetchBranches fetches from remote into repo, filtered by pattern:
//   - pattern == "": fetch everything, git fetch's own default behavior.
//   - pattern is a glob (e.g. "main", "release/*"): converted directly to a
//     custom refspec and passed to `git fetch` — no ls-remote call.
//   - pattern is a regex (e.g. "^(main|develop)$"): remoteURL is listed via
//     `git ls-remote`, matching branch names are filtered client-side, and
//     an explicit refspec per match is fetched.
func FetchBranches(ctx context.Context, c fetchClient, repo, remote, remoteURL, pattern string) (FetchReport, error) {
	if pattern == "" {
		if err := c.Fetch(ctx, repo, remote); err != nil {
			return FetchReport{}, fmt.Errorf("sync: fetch %q in %q: %w", remote, repo, err)
		}
		return FetchReport{Mode: "all"}, nil
	}

	if isRegexPattern(pattern) {
		return fetchByRegex(ctx, c, repo, remote, remoteURL, pattern)
	}
	return fetchByGlob(ctx, c, repo, remote, pattern)
}

func branchRefSpec(remote, branch string) string {
	return fmt.Sprintf("refs/heads/%s:refs/remotes/%s/%s", branch, remote, branch)
}

func fetchByGlob(ctx context.Context, c fetchClient, repo, remote, pattern string) (FetchReport, error) {
	refSpec := branchRefSpec(remote, pattern)
	if err := c.Fetch(ctx, repo, remote, refSpec); err != nil {
		return FetchReport{}, fmt.Errorf("sync: fetch %q (glob %q) in %q: %w", remote, pattern, repo, err)
	}
	return FetchReport{Mode: "glob", RefSpecs: []string{refSpec}}, nil
}

func fetchByRegex(ctx context.Context, c fetchClient, repo, remote, remoteURL, pattern string) (FetchReport, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return FetchReport{}, fmt.Errorf("sync: invalid regex branch pattern %q: %w", pattern, err)
	}

	out, err := c.LSRemote(ctx, remoteURL)
	if err != nil {
		return FetchReport{}, fmt.Errorf("sync: ls-remote %q: %w", remoteURL, err)
	}
	branches := matchingBranches(out, re)

	if len(branches) == 0 {
		return FetchReport{Mode: "regex"}, nil
	}

	refSpecs := make([]string, len(branches))
	for i, name := range branches {
		refSpecs[i] = branchRefSpec(remote, name)
	}
	if err := c.Fetch(ctx, repo, remote, refSpecs...); err != nil {
		return FetchReport{}, fmt.Errorf("sync: fetch %q (regex %q) in %q: %w", remote, pattern, repo, err)
	}
	return FetchReport{Mode: "regex", RefSpecs: refSpecs, Branches: branches}, nil
}

// matchingBranches parses `git ls-remote` output ("<hash>\t<ref>" lines),
// keeps only refs/heads/* entries, and returns the branch names (with the
// refs/heads/ prefix stripped) that re matches, sorted for a deterministic
// report.
func matchingBranches(lsRemoteOutput string, re *regexp.Regexp) []string {
	const headsPrefix = "refs/heads/"
	var branches []string
	for _, line := range strings.Split(lsRemoteOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], headsPrefix) {
			continue
		}
		name := strings.TrimPrefix(fields[1], headsPrefix)
		if re.MatchString(name) {
			branches = append(branches, name)
		}
	}
	sort.Strings(branches)
	return branches
}
