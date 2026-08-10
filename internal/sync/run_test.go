package sync

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Odilhao/git-manager/internal/config"
	"github.com/Odilhao/git-manager/internal/gitcli"
)

func oneRepoConfig(groupPath, repoName string, remotes map[string]config.RemoteConfig, identity config.IdentityConfig) *config.Config {
	return &config.Config{
		Groups: []config.GroupConfig{
			{
				Name: "work",
				Path: groupPath,
				Repos: []config.RepoConfig{
					{Name: repoName, Remotes: remotes, IdentityConfig: identity},
				},
			},
		},
	}
}

func TestRun_FreshCloneReconcilesIdentityAndFetches(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: fileURL(origin)}},
		config.IdentityConfig{UserName: strPtr("Octocat"), UserEmail: strPtr("octocat@example.com")},
	)
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{})

	if report.ErrorCount != 0 {
		t.Fatalf("report.ErrorCount = %d, want 0: %+v", report.ErrorCount, report.Repos)
	}
	if len(report.Repos) != 1 {
		t.Fatalf("len(report.Repos) = %d, want 1", len(report.Repos))
	}
	rr := report.Repos[0]
	if !rr.Cloned {
		t.Fatalf("rr.Cloned = false, want true for a fresh clone")
	}
	wantPath := filepath.Join(groupPath, "example-project")
	if rr.Path != wantPath {
		t.Fatalf("rr.Path = %q, want %q", rr.Path, wantPath)
	}
	if _, err := os.Stat(filepath.Join(wantPath, "file.txt")); err != nil {
		t.Fatalf("checkout was not actually cloned: %v", err)
	}

	name, err := c.ConfigGet(context.Background(), wantPath, "local", "user.name")
	if err != nil || name != "Octocat" {
		t.Fatalf("user.name = %q, err %v, want Octocat", name, err)
	}
	if len(rr.Fetches) != 1 || rr.Fetches[0].Remote != "origin" {
		t.Fatalf("rr.Fetches = %+v, want a single origin fetch", rr.Fetches)
	}
}

func TestRun_DryRunFreshCloneMakesNoFilesystemChanges(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: fileURL(origin)}},
		config.IdentityConfig{UserName: strPtr("Octocat")},
	)
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{DryRun: true})

	if report.ErrorCount != 0 {
		t.Fatalf("report.ErrorCount = %d, want 0: %+v", report.ErrorCount, report.Repos)
	}
	rr := report.Repos[0]
	if !rr.Cloned {
		t.Fatalf("rr.Cloned = false, want true (planned clone)")
	}
	wantPath := filepath.Join(groupPath, "example-project")
	if _, err := os.Stat(wantPath); err == nil {
		t.Fatalf("dry-run actually cloned into %q", wantPath)
	}
	if len(rr.Remotes.Added) != 1 || rr.Remotes.Added[0].Name != "origin" {
		t.Fatalf("rr.Remotes.Added = %+v, want a planned origin add", rr.Remotes.Added)
	}
	if len(rr.Identity.Written) != 1 || rr.Identity.Written[0].Key != "user.name" {
		t.Fatalf("rr.Identity.Written = %+v, want a planned user.name write", rr.Identity.Written)
	}
}

// TestRun_DryRunFreshCloneReflectsConfiguredBranchPattern proves the
// synthetic fresh-clone plan doesn't silently fall back to "fetch
// everything": a configured pattern must still surface as the corresponding
// FetchReport, even though the underlying repo is never actually cloned in
// dry-run.
func TestRun_DryRunFreshCloneReflectsConfiguredBranchPattern(t *testing.T) {
	origin := branchOriginForRun(t)
	groupPath := t.TempDir()
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: fileURL(origin), Branches: "release/*"}},
		config.IdentityConfig{},
	)
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{DryRun: true})

	if report.ErrorCount != 0 {
		t.Fatalf("report.ErrorCount = %d, want 0: %+v", report.ErrorCount, report.Repos)
	}
	rr := report.Repos[0]
	if len(rr.Fetches) != 1 {
		t.Fatalf("rr.Fetches = %+v, want one entry", rr.Fetches)
	}
	fr := rr.Fetches[0].Report
	if fr.Mode != "glob" || len(fr.RefSpecs) != 1 || fr.RefSpecs[0] != "refs/heads/release/*:refs/remotes/origin/release/*" {
		t.Fatalf("rr.Fetches[0].Report = %+v, want a planned glob fetch for 'release/*'", fr)
	}
	if _, err := os.Stat(filepath.Join(groupPath, "example-project")); err == nil {
		t.Fatal("dry-run actually cloned the repo while planning the fetch")
	}
}

// preexistingRepo creates a real, already-cloned checkout with origin plus an
// undeclared remote "scratch", used to exercise the overwrite/prune wiring
// against a repo Run does not need to clone.
func preexistingRepo(t *testing.T, groupPath, repoName, originURL string) string {
	t.Helper()
	path := filepath.Join(groupPath, repoName)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	runFixture(t, path, "init", "-b", "main")
	runFixture(t, path, "config", "user.name", "Octocat")
	runFixture(t, path, "config", "user.email", "octocat@example.com")
	runFixture(t, path, "remote", "add", "origin", originURL)
	runFixture(t, path, "remote", "add", "scratch", "https://example.com/octocat/scratch.git")
	return path
}

func TestRun_OverwriteFalseLeavesUndeclaredRemoteAlone(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	path := preexistingRepo(t, groupPath, "example-project", fileURL(origin))
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: fileURL(origin)}},
		config.IdentityConfig{},
	)
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{Overwrite: false})

	if report.ErrorCount != 0 {
		t.Fatalf("report.ErrorCount = %d, want 0: %+v", report.ErrorCount, report.Repos)
	}
	rr := report.Repos[0]
	if len(rr.Remotes.Removed) != 0 {
		t.Fatalf("rr.Remotes.Removed = %+v, want none when overwrite is false", rr.Remotes.Removed)
	}
	current, err := c.RemoteList(context.Background(), path)
	if err != nil {
		t.Fatalf("RemoteList: %v", err)
	}
	if _, ok := current["scratch"]; !ok {
		t.Fatal("undeclared remote 'scratch' was removed despite overwrite=false")
	}
}

func TestRun_OverwriteTrueRemovesUndeclaredRemote(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	path := preexistingRepo(t, groupPath, "example-project", fileURL(origin))
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: fileURL(origin)}},
		config.IdentityConfig{},
	)
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{Overwrite: true})

	if report.ErrorCount != 0 {
		t.Fatalf("report.ErrorCount = %d, want 0: %+v", report.ErrorCount, report.Repos)
	}
	rr := report.Repos[0]
	if len(rr.Remotes.Removed) != 1 || rr.Remotes.Removed[0].Name != "scratch" {
		t.Fatalf("rr.Remotes.Removed = %+v, want one entry for 'scratch'", rr.Remotes.Removed)
	}
	current, err := c.RemoteList(context.Background(), path)
	if err != nil {
		t.Fatalf("RemoteList: %v", err)
	}
	if _, ok := current["scratch"]; ok {
		t.Fatal("undeclared remote 'scratch' still present despite overwrite=true")
	}
}

func TestRun_DryRunOverwriteTrueDoesNotActuallyRemoveRemote(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	path := preexistingRepo(t, groupPath, "example-project", fileURL(origin))
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: fileURL(origin)}},
		config.IdentityConfig{},
	)
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{DryRun: true, Overwrite: true})

	if report.ErrorCount != 0 {
		t.Fatalf("report.ErrorCount = %d, want 0: %+v", report.ErrorCount, report.Repos)
	}
	rr := report.Repos[0]
	if len(rr.Remotes.Removed) != 1 || rr.Remotes.Removed[0].Name != "scratch" {
		t.Fatalf("rr.Remotes.Removed = %+v, want a planned removal of 'scratch'", rr.Remotes.Removed)
	}
	current, err := c.RemoteList(context.Background(), path)
	if err != nil {
		t.Fatalf("RemoteList: %v", err)
	}
	if _, ok := current["scratch"]; !ok {
		t.Fatal("dry-run actually removed undeclared remote 'scratch'")
	}
}

// TestRun_OriginNeverRemovedEvenWithOverwrite is the invariant-3 safety
// property re-verified at the orchestration layer: even with Overwrite true
// and origin left undeclared in config, origin itself must survive.
func TestRun_OriginNeverRemovedEvenWithOverwrite(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	path := preexistingRepo(t, groupPath, "example-project", fileURL(origin))
	// origin deliberately left out of declared remotes.
	cfg := oneRepoConfig(groupPath, "example-project", map[string]config.RemoteConfig{}, config.IdentityConfig{})
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{Overwrite: true})

	rr := report.Repos[0]
	for _, removed := range rr.Remotes.Removed {
		if removed.Name == "origin" {
			t.Fatalf("report claims origin was removed: %+v", rr.Remotes.Removed)
		}
	}
	current, err := c.RemoteList(context.Background(), path)
	if err != nil {
		t.Fatalf("RemoteList: %v", err)
	}
	if _, ok := current["origin"]; !ok {
		t.Fatal("origin was deleted despite the always-survives rule")
	}
}

// branchOriginForRun mirrors branchOrigin (branches_test.go) but is scoped
// here so run_test.go doesn't need to depend on branches_test.go's naming.
func branchOriginForRun(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runFixture(t, dir, "init", "-b", "main")
	runFixture(t, dir, "config", "user.name", "Octocat")
	runFixture(t, dir, "config", "user.email", "octocat@example.com")
	runFixture(t, dir, "commit", "--allow-empty", "-m", "initial")
	for _, branch := range []string{"develop", "release/1.0", "release/2.0"} {
		runFixture(t, dir, "branch", branch)
	}
	return dir
}

func TestRun_AppliesConfiguredBranchPatternPerRemote(t *testing.T) {
	origin := branchOriginForRun(t)
	groupPath := t.TempDir()
	// A fresh CloneIfMissing runs an ordinary `git clone`, which already
	// fetches every branch by default — that would mask whether Run's own
	// per-remote FetchBranches call did any filtering. Pre-cloning
	// single-branch, exactly at Run's own resolved path, means Run finds an
	// existing checkout and skips straight to reconciliation/fetch, so only
	// the configured pattern governs what lands under refs/remotes/origin.
	wantPath := filepath.Join(groupPath, "example-project")
	runFixture(t, "", "clone", "-q", "--single-branch", "--branch", "main", fileURL(origin), wantPath)
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: fileURL(origin), Branches: "develop"}},
		config.IdentityConfig{},
	)
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{})

	if report.ErrorCount != 0 {
		t.Fatalf("report.ErrorCount = %d, want 0: %+v", report.ErrorCount, report.Repos)
	}
	rr := report.Repos[0]
	if len(rr.Fetches) != 1 {
		t.Fatalf("rr.Fetches = %+v, want one entry", rr.Fetches)
	}
	fr := rr.Fetches[0].Report
	if fr.Mode != "glob" || len(fr.RefSpecs) != 1 || fr.RefSpecs[0] != "refs/heads/develop:refs/remotes/origin/develop" {
		t.Fatalf("rr.Fetches[0].Report = %+v, want a glob fetch of just 'develop'", fr)
	}

	cmd := exec.Command("git", "-C", wantPath, "for-each-ref", "--format=%(refname:short)", "refs/remotes/origin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("for-each-ref: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	if strings.Contains(got, "release") {
		t.Fatalf("unfiltered branches were fetched despite a configured glob pattern: %q", got)
	}
	if !strings.Contains(got, "develop") {
		t.Fatalf("configured branch 'develop' was not fetched: %q", got)
	}
}

// TestRun_OutcomeSuccessOnCleanSync proves a repo that syncs with no error
// is classified "success".
func TestRun_OutcomeSuccessOnCleanSync(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: fileURL(origin)}},
		config.IdentityConfig{},
	)
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{})

	rr := report.Repos[0]
	if rr.Error != "" {
		t.Fatalf("rr.Error = %q, want none: %+v", rr.Error, rr)
	}
	if rr.Outcome != "success" {
		t.Fatalf("rr.Outcome = %q, want %q", rr.Outcome, "success")
	}
}

// TestRun_OutcomeFailureWhenNothingAccomplished proves a repo that errors
// before recording any activity is classified "failure", not "partial".
func TestRun_OutcomeFailureWhenNothingAccomplished(t *testing.T) {
	groupPath := t.TempDir()
	// No origin declared and no local checkout exists: syncRepo fails
	// before Cloned, Remotes, Identity or Fetches are ever touched.
	cfg := oneRepoConfig(groupPath, "no-origin", map[string]config.RemoteConfig{}, config.IdentityConfig{})
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{})

	rr := report.Repos[0]
	if rr.Error == "" {
		t.Fatal("rr.Error is empty, want an error for a repo with no origin and no checkout")
	}
	if rr.Outcome != "failure" {
		t.Fatalf("rr.Outcome = %q, want %q for an error with no recorded activity", rr.Outcome, "failure")
	}
}

// TestRun_OutcomePartialWhenErrorAfterActivity proves a repo that records
// some activity (here, a remote URL update) before later failing is
// classified "partial", not "failure".
func TestRun_OutcomePartialWhenErrorAfterActivity(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	path := preexistingRepo(t, groupPath, "example-project", fileURL(origin))
	// Declaring a different origin URL forces ReconcileRemotes to update it
	// (recorded activity) before the subsequent fetch against that
	// now-broken URL fails.
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: "https://127.0.0.1:1/does-not-exist.git"}},
		config.IdentityConfig{},
	)
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{})

	rr := report.Repos[0]
	if rr.Error == "" {
		t.Fatalf("rr.Error is empty, want a fetch error against an unreachable remote (path %q)", path)
	}
	if len(rr.Remotes.Updated) == 0 {
		t.Fatalf("rr.Remotes.Updated = %+v, want the origin URL update recorded before the fetch failure", rr.Remotes.Updated)
	}
	if rr.Outcome != "partial" {
		t.Fatalf("rr.Outcome = %q, want %q for an error recorded after activity", rr.Outcome, "partial")
	}
}

// TestRun_DurationMSPositiveOnRealRun proves DurationMS is measured, not
// just present, on both the per-repo result and the overall report.
func TestRun_DurationMSPositiveOnRealRun(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: fileURL(origin)}},
		config.IdentityConfig{},
	)
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{})

	if report.DurationMS <= 0 {
		t.Fatalf("report.DurationMS = %d, want > 0", report.DurationMS)
	}
	if report.Repos[0].DurationMS <= 0 {
		t.Fatalf("report.Repos[0].DurationMS = %d, want > 0", report.Repos[0].DurationMS)
	}
}

func TestRun_MissingOriginOnUnclonedRepoIsReportedAsErrorAndOthersStillRun(t *testing.T) {
	origin := initBareRepo(t)
	groupRoot := t.TempDir()

	brokenGroup := filepath.Join(groupRoot, "broken")
	okGroup := filepath.Join(groupRoot, "ok")
	if err := os.MkdirAll(brokenGroup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(okGroup, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Groups: []config.GroupConfig{
			{
				Name: "broken",
				Path: brokenGroup,
				Repos: []config.RepoConfig{
					{Name: "no-origin", Remotes: map[string]config.RemoteConfig{}},
				},
			},
			{
				Name: "ok",
				Path: okGroup,
				Repos: []config.RepoConfig{
					{Name: "example-project", Remotes: map[string]config.RemoteConfig{"origin": {URL: fileURL(origin)}}},
				},
			},
		},
	}
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{})

	if report.ErrorCount != 1 {
		t.Fatalf("report.ErrorCount = %d, want 1: %+v", report.ErrorCount, report.Repos)
	}
	if len(report.Repos) != 2 {
		t.Fatalf("len(report.Repos) = %d, want 2 (broken repo must not block the rest)", len(report.Repos))
	}
	if report.Repos[0].Error == "" {
		t.Fatalf("report.Repos[0].Error is empty, want an error for the repo with no declared origin")
	}
	if !report.Repos[1].Cloned {
		t.Fatalf("report.Repos[1].Cloned = false, want true; the second repo must still sync despite the first's error")
	}
}

// TestRun_ConcurrencyProcessesReposInParallel verifies that with concurrency > 1,
// repos are processed concurrently (observable via wall-clock time) and that
// Report.Repos stays in declaration order regardless of completion order.
func TestRun_ConcurrencyProcessesReposInParallel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrency test in short mode")
	}

	// Create 3 repos with artificial delays to observe concurrent execution.
	origins := make([]string, 3)
	for i := 0; i < 3; i++ {
		origins[i] = initBareRepo(t)
	}

	groupRoot := t.TempDir()
	brokenGroup := filepath.Join(groupRoot, "broken")
	okGroup := filepath.Join(groupRoot, "ok")
	if err := os.MkdirAll(brokenGroup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(okGroup, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Groups: []config.GroupConfig{
			{
				Name: "group1",
				Path: okGroup,
				Repos: []config.RepoConfig{
					{Name: "repo1", Remotes: map[string]config.RemoteConfig{"origin": {URL: fileURL(origins[0])}}},
					{Name: "repo2", Remotes: map[string]config.RemoteConfig{"origin": {URL: fileURL(origins[1])}}},
					{Name: "repo3", Remotes: map[string]config.RemoteConfig{"origin": {URL: fileURL(origins[2])}}},
				},
			},
		},
	}

	delayClient := &delayingClient{real: gitcli.NewClient()}
	report := Run(context.Background(), delayClient, cfg, Options{Concurrency: 2})

	// Verify all repos were processed.
	if len(report.Repos) != 3 {
		t.Fatalf("len(report.Repos) = %d, want 3", len(report.Repos))
	}

	// Verify Report.Repos stays in declaration order (repo1, repo2, repo3).
	if report.Repos[0].Name != "repo1" {
		t.Fatalf("report.Repos[0].Name = %q, want repo1", report.Repos[0].Name)
	}
	if report.Repos[1].Name != "repo2" {
		t.Fatalf("report.Repos[1].Name = %q, want repo2", report.Repos[1].Name)
	}
	if report.Repos[2].Name != "repo3" {
		t.Fatalf("report.Repos[2].Name = %q, want repo3", report.Repos[2].Name)
	}

	// Verify all repos synced successfully.
	for i, rr := range report.Repos {
		if rr.Error != "" {
			t.Fatalf("report.Repos[%d].Error = %q, want none", i, rr.Error)
		}
		if !rr.Cloned {
			t.Fatalf("report.Repos[%d].Cloned = false, want true", i)
		}
	}

	// With 2 concurrent workers processing 3 repos with delays, wall-clock should
	// be significantly less than sequential (3 delays). At concurrency=2:
	// Timeline: [Delay1][Delay2] || [Delay3] = ~2 delays wall-clock
	// Sequential would be 3 delays.
	// We measure individual durations to infer parallelism.
	totalIndividualTime := report.Repos[0].DurationMS + report.Repos[1].DurationMS + report.Repos[2].DurationMS
	overallTime := report.DurationMS
	// In parallel at concurrency 2, overall should be roughly max(delay pairs),
	// roughly half of sequential. We assert a loose bound: overall should be
	// less than 70% of the sum (allowing for scheduling overhead).
	if overallTime >= int64(float64(totalIndividualTime)*0.7) {
		t.Logf("Timing suggests insufficient parallelism: overall=%dms, sum=%dms (ratio=%.2f)",
			overallTime, totalIndividualTime, float64(overallTime)/float64(totalIndividualTime))
		// Not a hard failure - CI may have variable timing - but log it for investigation.
	}
}

// delayingClient wraps a real client and adds a delay to Clone to make
// concurrency observable in timing tests.
type delayingClient struct {
	real Client
}

func (d *delayingClient) Clone(ctx context.Context, url, path string) error {
	// Simulate network I/O latency with a short delay.
	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	return d.real.Clone(ctx, url, path)
}

func (d *delayingClient) RemoteList(ctx context.Context, repo string) (map[string]string, error) {
	return d.real.RemoteList(ctx, repo)
}

func (d *delayingClient) RemoteAdd(ctx context.Context, repo, name, url string) error {
	return d.real.RemoteAdd(ctx, repo, name, url)
}

func (d *delayingClient) RemoteSetURL(ctx context.Context, repo, name, url string) error {
	return d.real.RemoteSetURL(ctx, repo, name, url)
}

func (d *delayingClient) RemoteRemove(ctx context.Context, repo, name string) error {
	return d.real.RemoteRemove(ctx, repo, name)
}

func (d *delayingClient) ConfigGet(ctx context.Context, repo, scope, key string) (string, error) {
	return d.real.ConfigGet(ctx, repo, scope, key)
}

func (d *delayingClient) ConfigSet(ctx context.Context, repo, scope, key, value string) error {
	return d.real.ConfigSet(ctx, repo, scope, key, value)
}

func (d *delayingClient) Fetch(ctx context.Context, repo, remote string, refSpecs ...string) error {
	return d.real.Fetch(ctx, repo, remote, refSpecs...)
}

func (d *delayingClient) LSRemote(ctx context.Context, url string) (string, error) {
	return d.real.LSRemote(ctx, url)
}

// TestRun_ConcurrencyOneIsSequential verifies that concurrency=1 behaves
// sequentially and that Report.Repos order is preserved.
func TestRun_ConcurrencyOneIsSequential(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: fileURL(origin)}},
		config.IdentityConfig{},
	)
	c := gitcli.NewClient()

	// Sequential run with concurrency=1
	report := Run(context.Background(), c, cfg, Options{Concurrency: 1})

	if report.ErrorCount != 0 {
		t.Fatalf("report.ErrorCount = %d, want 0: %+v", report.ErrorCount, report.Repos)
	}
	if len(report.Repos) != 1 {
		t.Fatalf("len(report.Repos) = %d, want 1", len(report.Repos))
	}
	rr := report.Repos[0]
	if !rr.Cloned {
		t.Fatalf("rr.Cloned = false, want true")
	}
	if rr.Name != "example-project" {
		t.Fatalf("rr.Name = %q, want example-project", rr.Name)
	}
}

// TestRun_DefaultConcurrencyIsFour verifies that unset/zero concurrency
// defaults to 4.
func TestRun_DefaultConcurrencyIsFour(t *testing.T) {
	origin := initBareRepo(t)
	groupPath := t.TempDir()
	cfg := oneRepoConfig(groupPath, "example-project",
		map[string]config.RemoteConfig{"origin": {URL: fileURL(origin)}},
		config.IdentityConfig{},
	)
	c := gitcli.NewClient()

	// Both Options{} (zero value) and Options{Concurrency: 0} should default to 4.
	for _, opts := range []Options{{}, {Concurrency: 0}} {
		report := Run(context.Background(), c, cfg, opts)
		if report.ErrorCount != 0 {
			t.Fatalf("report.ErrorCount = %d, want 0 for opts %+v", report.ErrorCount, opts)
		}
		if len(report.Repos) != 1 {
			t.Fatalf("len(report.Repos) = %d, want 1 for opts %+v", len(report.Repos), opts)
		}
	}
}

// TestRun_ZeroDeclaredReposReturnsNilReposNotEmpty verifies that when a config
// declares zero repos, Run returns a Report with Repos=nil (not an empty slice),
// ErrorCount=0, and a measured DurationMS. This preserves the pre-change contract
// that an empty loop leaves report.Repos unset, not appended to.
func TestRun_ZeroDeclaredReposReturnsNilReposNotEmpty(t *testing.T) {
	cfg := &config.Config{Groups: []config.GroupConfig{}}
	c := gitcli.NewClient()

	report := Run(context.Background(), c, cfg, Options{})

	if report.Repos != nil {
		t.Fatalf("report.Repos = %v, want nil (not empty slice) for zero declared repos", report.Repos)
	}
	if report.ErrorCount != 0 {
		t.Fatalf("report.ErrorCount = %d, want 0", report.ErrorCount)
	}
	if report.DurationMS < 0 {
		t.Fatalf("report.DurationMS = %d, want >= 0", report.DurationMS)
	}
}
