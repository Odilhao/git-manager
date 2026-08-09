package sync

import (
	"context"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"github.com/Odilhao/git-manager/internal/gitcli"
)

// branchOrigin creates a real git repo with several branches, suitable as a
// fetch source for FetchBranches tests.
func branchOrigin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runFixture(t, dir, "init", "-b", "main")
	runFixture(t, dir, "config", "user.name", "Octocat")
	runFixture(t, dir, "config", "user.email", "octocat@example.com")
	runFixture(t, dir, "commit", "--allow-empty", "-m", "initial")
	for _, branch := range []string{"feature-a", "feature-b", "release/1.0", "release/2.0", "develop"} {
		runFixture(t, dir, "branch", branch)
	}
	// A tag whose name would satisfy an unanchored "contains feature"
	// regex if matchingBranches ever stopped filtering to refs/heads/* —
	// see TestFetchBranches_RegexIgnoresNonBranchRefs.
	runFixture(t, dir, "tag", "feature-tag")
	return dir
}

// singleBranchClone clones only origin's default branch, so the checkout
// starts without any of the other branches FetchBranches tests select from —
// a default `git clone` already fetches every branch, which would mask
// whether FetchBranches's own filtering actually did anything.
func singleBranchClone(t *testing.T, origin string) string {
	t.Helper()
	dst := t.TempDir()
	runFixture(t, "", "clone", "-q", "--single-branch", "--branch", "main", fileURL(origin), dst)
	return dst
}

// localRemoteBranches lists what actually landed under refs/remotes/<remote>
// in repo, independent of FetchBranches's own report — the ground truth a
// test checks the report against.
func localRemoteBranches(t *testing.T, repo, remote string) []string {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "for-each-ref", "--format=%(refname:short)", "refs/remotes/"+remote)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("for-each-ref: %v\n%s", err, out)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		// refs/remotes/<remote>/HEAD shortens to the bare remote name
		// ("origin"), a symbolic ref this package never fetches directly —
		// exclude it so tests assert only on actual branches.
		if line != "" && line != remote {
			names = append(names, line)
		}
	}
	sort.Strings(names)
	return names
}

func TestFetchBranches_EmptyPatternFetchesEverything(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	// singleBranchClone leaves remote.origin.fetch scoped to just "main" (a
	// --single-branch clone's own config), which would make "fetch
	// everything" trivially pass by fetching the single branch it already
	// had. Widen it back to the ordinary all-branches refspec so this test
	// actually exercises FetchBranches's own "" => fetch everything path.
	runFixture(t, dst, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "")
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	if report.Mode != "all" {
		t.Fatalf("report.Mode = %q, want %q", report.Mode, "all")
	}

	got := localRemoteBranches(t, dst, "origin")
	want := []string{"origin/develop", "origin/feature-a", "origin/feature-b", "origin/main", "origin/release/1.0", "origin/release/2.0"}
	assertStringSlicesEqual(t, got, want)
}

func TestFetchBranches_EmptyPatternSurfacesFetchError(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	// Point origin at a URL that no longer exists so the underlying `git
	// fetch` fails, proving FetchBranches's "" (fetch everything) path
	// surfaces that failure rather than swallowing it.
	runFixture(t, dst, "remote", "set-url", "origin", fileURL(origin+"-does-not-exist"))
	c := gitcli.NewClient()

	_, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "")
	if err == nil {
		t.Fatal("expected an error when the underlying fetch fails, got nil")
	}
}

func TestFetchBranches_GlobLiteralFetchesOnlyThatBranch(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "develop")
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	if report.Mode != "glob" {
		t.Fatalf("report.Mode = %q, want %q", report.Mode, "glob")
	}
	if len(report.RefSpecs) != 1 || report.RefSpecs[0] != "refs/heads/develop:refs/remotes/origin/develop" {
		t.Fatalf("report.RefSpecs = %v, want a single develop refspec", report.RefSpecs)
	}

	got := localRemoteBranches(t, dst, "origin")
	want := []string{"origin/develop", "origin/main"}
	assertStringSlicesEqual(t, got, want)
}

func TestFetchBranches_GlobSurfacesFetchError(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	runFixture(t, dst, "remote", "set-url", "origin", fileURL(origin+"-does-not-exist"))
	c := gitcli.NewClient()

	_, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "develop")
	if err == nil {
		t.Fatal("expected an error when the underlying glob fetch fails, got nil")
	}
}

func TestFetchBranches_GlobWildcardFetchesMatchingBranches(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "release/*")
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	if report.Mode != "glob" {
		t.Fatalf("report.Mode = %q, want %q", report.Mode, "glob")
	}
	if len(report.Branches) != 0 {
		t.Fatalf("report.Branches = %v, glob mode must not enumerate matched branches (no ls-remote call)", report.Branches)
	}

	got := localRemoteBranches(t, dst, "origin")
	want := []string{"origin/main", "origin/release/1.0", "origin/release/2.0"}
	assertStringSlicesEqual(t, got, want)
}

func TestFetchBranches_RegexFetchesOnlyMatchingBranches(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "^(main|develop)$")
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	if report.Mode != "regex" {
		t.Fatalf("report.Mode = %q, want %q", report.Mode, "regex")
	}
	assertStringSlicesEqual(t, report.Branches, []string{"develop", "main"})

	got := localRemoteBranches(t, dst, "origin")
	want := []string{"origin/develop", "origin/main"}
	assertStringSlicesEqual(t, got, want)
}

// TestFetchBranches_CaretAloneSelectsRegexMode isolates "^" as the sole
// regex-only signal in the pattern (no "$", "|", "(", ")" or "\") — proving
// isRegexPattern doesn't rely on the other regex-only characters also being
// present to route correctly.
func TestFetchBranches_CaretAloneSelectsRegexMode(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "^feature-.*")
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	if report.Mode != "regex" {
		t.Fatalf("report.Mode = %q, want %q", report.Mode, "regex")
	}
	assertStringSlicesEqual(t, report.Branches, []string{"feature-a", "feature-b"})

	got := localRemoteBranches(t, dst, "origin")
	want := []string{"origin/feature-a", "origin/feature-b", "origin/main"}
	assertStringSlicesEqual(t, got, want)
}

// TestFetchBranches_DollarAloneSelectsRegexMode isolates "$" as the sole
// regex-only signal present (no "^", "|", "(", ")", "+" or "\") — mirrors
// TestFetchBranches_CaretAloneSelectsRegexMode for the next member of
// regexOnlyChars.
func TestFetchBranches_DollarAloneSelectsRegexMode(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "develop$")
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	if report.Mode != "regex" {
		t.Fatalf("report.Mode = %q, want %q", report.Mode, "regex")
	}
	assertStringSlicesEqual(t, report.Branches, []string{"develop"})
}

// TestFetchBranches_PipeAloneSelectsRegexMode isolates "|" as the sole
// regex-only signal present.
func TestFetchBranches_PipeAloneSelectsRegexMode(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "zzznomatch|feature-a")
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	if report.Mode != "regex" {
		t.Fatalf("report.Mode = %q, want %q", report.Mode, "regex")
	}
	assertStringSlicesEqual(t, report.Branches, []string{"feature-a"})
}

// TestFetchBranches_PlusAloneSelectsRegexMode isolates "+" as the sole
// regex-only signal present.
func TestFetchBranches_PlusAloneSelectsRegexMode(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "feature+")
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	if report.Mode != "regex" {
		t.Fatalf("report.Mode = %q, want %q", report.Mode, "regex")
	}
	assertStringSlicesEqual(t, report.Branches, []string{"feature-a", "feature-b"})
}

// TestFetchBranches_BackslashAloneSelectsRegexMode isolates "\" as the sole
// regex-only signal present: `feature\-a` is an escaped literal hyphen,
// equivalent to "feature-a" as a regex, and matches nothing by accident.
func TestFetchBranches_BackslashAloneSelectsRegexMode(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), `feature\-a`)
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	if report.Mode != "regex" {
		t.Fatalf("report.Mode = %q, want %q", report.Mode, "regex")
	}
	assertStringSlicesEqual(t, report.Branches, []string{"feature-a"})
}

// TestFetchBranches_OpenParenAloneSelectsRegexMode and
// TestFetchBranches_CloseParenAloneSelectsRegexMode isolate "(" and ")"
// respectively. Neither can appear alone in a *valid* regex (parens must be
// balanced), so instead of asserting a successful match, each asserts the
// pattern was routed into fetchByRegex at all: only regex mode calls
// regexp.Compile, so only regex mode can produce this specific "invalid
// regex branch pattern" wrapped error. If either character were dropped
// from regexOnlyChars, the pattern would be treated as a literal glob
// branch name instead — fetchByGlob never calls regexp.Compile, so it would
// either succeed (treating the parenthesis as a literal, if git's ref-name
// rules allow it) or fail with a *different* error shape (git's own
// "invalid refspec"/"couldn't find remote ref"), never this one.
func TestFetchBranches_OpenParenAloneSelectsRegexMode(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	_, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "feature(a")
	if err == nil || !strings.Contains(err.Error(), "invalid regex branch pattern") {
		t.Fatalf("FetchBranches error = %v, want it to name an invalid regex pattern (proving regex mode ran)", err)
	}
}

func TestFetchBranches_CloseParenAloneSelectsRegexMode(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	_, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "featurea)")
	if err == nil || !strings.Contains(err.Error(), "invalid regex branch pattern") {
		t.Fatalf("FetchBranches error = %v, want it to name an invalid regex pattern (proving regex mode ran)", err)
	}
}

// TestFetchBranches_RegexIgnoresNonBranchRefs proves matchingBranches only
// considers refs/heads/* entries from ls-remote: an unanchored pattern that
// would otherwise match the "feature-tag" tag (a substring match against its
// full ref path) must not select it, since it isn't a branch.
func TestFetchBranches_RegexIgnoresNonBranchRefs(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "feature|nonexistent$")
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	assertStringSlicesEqual(t, report.Branches, []string{"feature-a", "feature-b"})
}

func TestFetchBranches_RegexNoMatchesFetchesNothing(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "^nonexistent$")
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	if report.Mode != "regex" || len(report.Branches) != 0 || len(report.RefSpecs) != 0 {
		t.Fatalf("report = %+v, want regex mode with zero matches and no fetch", report)
	}

	got := localRemoteBranches(t, dst, "origin")
	want := []string{"origin/main"}
	assertStringSlicesEqual(t, got, want)
}

// TestFetchBranches_RegexNoMatchesDoesNotFallBackToFetchingEverything closes
// a specific gap the zero-matches early return guards against: a naive
// `Fetch(ctx, repo, remote, refSpecs...)` call with an empty refSpecs slice
// behaves identically, in Go, to calling Fetch with no refspecs at all —
// which is `git fetch`'s "fetch everything the configured refspec covers"
// default. Widening remote.origin.fetch to the ordinary all-branches refspec
// (as opposed to singleBranchClone's own scoped-to-main one) makes that
// fallback observable: if FetchBranches ever skipped its zero-match guard,
// every branch would land, not none.
func TestFetchBranches_RegexNoMatchesDoesNotFallBackToFetchingEverything(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	runFixture(t, dst, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	c := gitcli.NewClient()

	report, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "^nonexistent$")
	if err != nil {
		t.Fatalf("FetchBranches: %v", err)
	}
	if len(report.Branches) != 0 || len(report.RefSpecs) != 0 {
		t.Fatalf("report = %+v, want zero matches and no refspecs", report)
	}

	got := localRemoteBranches(t, dst, "origin")
	want := []string{"origin/main"}
	assertStringSlicesEqual(t, got, want)
}

func TestFetchBranches_RegexSurfacesLSRemoteError(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	_, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin+"-does-not-exist"), "^(main|develop)$")
	if err == nil {
		t.Fatal("expected an error when ls-remote against a bad URL fails, got nil")
	}
}

func TestFetchBranches_RegexSurfacesFetchError(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	// ls-remote still targets the real origin (so matching succeeds and
	// selects real branches), but the configured "origin" remote itself
	// points nowhere, so the subsequent `git fetch` fails.
	runFixture(t, dst, "remote", "set-url", "origin", fileURL(origin+"-does-not-exist"))
	c := gitcli.NewClient()

	_, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "^(main|develop)$")
	if err == nil {
		t.Fatal("expected an error when the underlying regex-mode fetch fails, got nil")
	}
}

func TestFetchBranches_InvalidRegexIsRejected(t *testing.T) {
	origin := branchOrigin(t)
	dst := singleBranchClone(t, origin)
	c := gitcli.NewClient()

	_, err := FetchBranches(context.Background(), c, dst, "origin", fileURL(origin), "^release-(unterminated")
	if err == nil {
		t.Fatal("expected an error for an invalid regex pattern, got nil")
	}
}

func assertStringSlicesEqual(t *testing.T, got, want []string) {
	t.Helper()
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
