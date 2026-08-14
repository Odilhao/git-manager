package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Odilhao/git-manager/internal/config"
)

// Client is the full set of gitcli operations Run needs — the union of every
// step's own narrower interface.
type Client interface {
	gitClient
	remoteClient
	fetchClient
	identityClient
}

// RepoEvent carries the completion event for a single repo's sync.
type RepoEvent struct {
	Name   string
	Group  string
	Result RepoResult
}

// Options controls how Run treats every repo it syncs.
type Options struct {
	// DryRun reports what would change without running any git command that
	// could mutate a checkout, a remote or its config.
	DryRun bool
	// Overwrite removes an undeclared remote during reconciliation (the
	// project's --overwrite/--prune flags both map to this one behavior).
	// origin is never removed, declared or not.
	Overwrite bool
	// Concurrency is the number of repos to sync in parallel. If 0 or negative,
	// defaults to 4.
	Concurrency int
	// ProgressCallback, if non-nil, is invoked once per repo as it completes
	// (after its sync work finishes). It must not block or mutate shared state
	// beyond a simple format-and-print, as it is invoked concurrently from
	// worker goroutines.
	ProgressCallback func(RepoEvent)
	// ConfigPath is the resolved path to the config file that drove this run.
	// It is included in the report for traceability.
	ConfigPath string
}

// FetchResult pairs one declared remote with what fetching it did or, in
// dry-run, would do.
type FetchResult struct {
	Remote string      `json:"remote"`
	Report FetchReport `json:"report"`
}

// RepoResult reports the outcome of syncing (or, in dry-run, planning to
// sync) one repo. Error is set, and every other field left at its zero
// value, when the repo could not be synced at all (e.g. no origin declared
// for a checkout that doesn't exist yet).
//
// JSON shape is additive/forward-compatible; see docs/json-schema.md.
type RepoResult struct {
	Name     string         `json:"name"`
	Group    string         `json:"group,omitempty"`
	Path     string         `json:"path"`
	Cloned   bool           `json:"cloned"`
	Remotes  RemoteReport   `json:"remotes"`
	Identity IdentityReport `json:"identity"`
	Fetches  []FetchResult  `json:"fetches,omitempty"`
	// Outcome is derived from Error and the activity fields above, never
	// set independently — "success" (no error), "partial" (error set but
	// some activity was already recorded) or "failure" (error set, nothing
	// accomplished).
	Outcome string `json:"outcome"`
	// DurationMS is the wall-clock time syncRepo spent on this repo, in
	// milliseconds.
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// Report is Run's complete result: one RepoResult per repo declared in the
// config, in declaration order.
//
// JSON shape is additive/forward-compatible; see docs/json-schema.md.
type Report struct {
	Config     string       `json:"config"`
	Repos      []RepoResult `json:"repos"`
	DryRun     bool         `json:"dry_run"`
	Overwrite  bool         `json:"overwrite"`
	ErrorCount int          `json:"error_count"`
	// DurationMS is the wall-clock time Run spent overall, in milliseconds —
	// not a sum of the per-repo durations, so it stays correct if Run ever
	// syncs repos concurrently.
	DurationMS int64 `json:"duration_ms"`
}

// Run syncs every repo declared in cfg: resolves its local path, clones it
// if missing, reconciles its remotes, applies its identity/signing config,
// and fetches from each declared remote — in that order. One repo's error is
// recorded on its RepoResult rather than aborting the rest. Repos are synced
// concurrently up to opts.Concurrency workers (defaulting to 4 if unset or ≤ 0).
// Report.Repos is returned in declaration order, independent of completion order.
func Run(ctx context.Context, real Client, cfg *config.Config, opts Options) Report {
	start := time.Now()
	resolved, _ := cfg.Resolve() // Resolve never actually returns a non-nil error today.

	report := Report{Config: opts.ConfigPath, DryRun: opts.DryRun, Overwrite: opts.Overwrite}

	// Count total repos to pre-allocate results slice (no append races, deterministic order).
	totalRepos := 0
	for _, g := range cfg.Groups {
		totalRepos += len(g.Repos)
	}

	if totalRepos == 0 {
		report.DurationMS = time.Since(start).Milliseconds()
		return report
	}

	// Pre-allocate results indexed by declaration order.
	results := make([]RepoResult, totalRepos)

	// Determine effective concurrency: default to 4 if unset or ≤ 0.
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}

	// Semaphore: buffered channel of size concurrency acts as a token bucket.
	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	// Launch goroutines for each repo, respecting the concurrency limit.
	repoIdx := 0
	for _, g := range cfg.Groups {
		groupName := g.Name // Capture group name for the progress callback.
		for _, r := range g.Repos {
			idx := repoIdx // Capture for the goroutine.
			rr := resolved[idx]
			repoName := r.Name // Capture for the progress callback.
			repoIdx++

			wg.Add(1)
			go func() {
				defer wg.Done()
				// Acquire a semaphore token; block if at capacity.
				semaphore <- struct{}{}
				defer func() { <-semaphore }() // Release the token.
				result := syncRepo(ctx, real, g.Path, repoName, groupName, rr, opts)
				results[idx] = result
				if opts.ProgressCallback != nil {
					opts.ProgressCallback(RepoEvent{Name: repoName, Group: groupName, Result: result})
				}
			}()
		}
	}

	wg.Wait()
	report.Repos = results

	// Count errors now that all goroutines have finished (no races).
	for _, r := range results {
		if r.Error != "" {
			report.ErrorCount++
		}
	}

	report.DurationMS = time.Since(start).Milliseconds()
	return report
}

func syncRepo(ctx context.Context, real Client, groupPath, repoName, groupName string, rr config.ResolvedRepo, opts Options) (result RepoResult) {
	start := time.Now()
	result = RepoResult{Name: repoName, Group: groupName}
	defer func() {
		result.DurationMS = time.Since(start).Milliseconds()
		result.Outcome = computeOutcome(result)
	}()

	path, err := ResolveRepoPath(groupPath, repoName)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Path = path

	client := real
	if opts.DryRun {
		client = &dryRunClient{real: real}
	}

	preExisted := pathExists(path)
	if originCfg, hasOrigin := rr.Remotes[originRemoteName]; hasOrigin {
		if err := CloneIfMissing(ctx, client, path, originCfg.URL); err != nil {
			result.Error = err.Error()
			return result
		}
		result.Cloned = !preExisted
	} else if !preExisted {
		result.Error = fmt.Sprintf("sync: repo %q has no origin remote declared and no local checkout exists to clone", repoName)
		return result
	} else if !isValidGitRepo(path) {
		result.Error = (&NotAGitRepoError{Path: path}).Error()
		return result
	}

	// A dry-run clone is only recorded, never actually performed, so path
	// still doesn't exist on disk — RemoteList/ConfigGet would fail against
	// it. Report the plan for a fresh checkout instead of trying to inspect
	// one that was never created. FetchBranches is safe to call for real
	// here even so: its only checkout-path use is Fetch, which client
	// no-ops in dry-run, and its regex path's LSRemote reads the remote URL
	// directly, never the local path — so the reported plan reflects each
	// remote's actual configured branch pattern.
	if opts.DryRun && result.Cloned {
		result.Remotes = plannedRemotes(rr.Remotes)
		result.Identity = plannedIdentity(rr.Identity)
		for _, name := range sortedRemoteNames(rr.Remotes) {
			rc := rr.Remotes[name]
			fr, err := FetchBranches(ctx, client, path, name, rc.URL, rc.Branches)
			if err != nil {
				result.Error = err.Error()
				return result
			}
			result.Fetches = append(result.Fetches, FetchResult{Remote: name, Report: fr})
		}
		return result
	}

	declared := make(map[string]string, len(rr.Remotes))
	for name, rc := range rr.Remotes {
		declared[name] = rc.URL
	}
	remotesReport, err := ReconcileRemotes(ctx, client, path, declared, opts.Overwrite)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Remotes = remotesReport

	identityReport, err := ApplyIdentity(ctx, client, path, repoName, groupName, rr.Identity)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Identity = identityReport

	for _, name := range sortedRemoteNames(rr.Remotes) {
		rc := rr.Remotes[name]
		fr, err := FetchBranches(ctx, client, path, name, rc.URL, rc.Branches)
		if err != nil {
			result.Error = err.Error()
			return result
		}
		result.Fetches = append(result.Fetches, FetchResult{Remote: name, Report: fr})
	}

	return result
}

// computeOutcome derives a RepoResult's Outcome from Error and the activity
// already recorded on it — never an independently-set field, so it can't
// drift from what the result actually says happened.
func computeOutcome(r RepoResult) string {
	if r.Error == "" {
		return "success"
	}
	if r.Cloned || len(r.Remotes.Added) > 0 || len(r.Remotes.Updated) > 0 ||
		len(r.Identity.Written) > 0 || len(r.Fetches) > 0 {
		return "partial"
	}
	return "failure"
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isValidGitRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func plannedRemotes(declared map[string]config.RemoteConfig) RemoteReport {
	var report RemoteReport
	for _, name := range sortedRemoteNames(declared) {
		report.Added = append(report.Added, RemoteChange{Name: name, URL: declared[name].URL})
	}
	return report
}

func plannedIdentity(identity config.ResolvedIdentity) IdentityReport {
	var report IdentityReport
	if identity.UserName != nil {
		report.Written = append(report.Written, IdentityWrite{Key: "user.name", Value: *identity.UserName})
	}
	if identity.UserEmail != nil {
		report.Written = append(report.Written, IdentityWrite{Key: "user.email", Value: *identity.UserEmail})
	}
	if identity.SigningKey != nil {
		report.Written = append(report.Written, IdentityWrite{Key: "user.signingkey", Value: *identity.SigningKey})
	}
	if identity.SigningMethod != nil {
		if key, value, err := signingMethodConfig(*identity.SigningMethod); err == nil {
			report.Written = append(report.Written, IdentityWrite{Key: key, Value: value})
		}
	}
	return report
}

func sortedRemoteNames(m map[string]config.RemoteConfig) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// dryRunClient forwards every read to the real client but records rather
// than executes every write, so plan/apply/fetch functions can run for real
// (producing an accurate report) against an already-existing checkout
// without mutating it.
type dryRunClient struct {
	real Client
}

func (d *dryRunClient) Clone(ctx context.Context, url, path string) error {
	return nil
}

func (d *dryRunClient) RemoteList(ctx context.Context, repo string) (map[string]string, error) {
	return d.real.RemoteList(ctx, repo)
}

func (d *dryRunClient) RemoteAdd(ctx context.Context, repo, name, url string) error {
	return nil
}

func (d *dryRunClient) RemoteSetURL(ctx context.Context, repo, name, url string) error {
	return nil
}

func (d *dryRunClient) RemoteRemove(ctx context.Context, repo, name string) error {
	return nil
}

func (d *dryRunClient) ConfigGet(ctx context.Context, repo, scope, key string) (string, error) {
	return d.real.ConfigGet(ctx, repo, scope, key)
}

func (d *dryRunClient) ConfigSet(ctx context.Context, repo, scope, key, value string) error {
	return nil
}

func (d *dryRunClient) Fetch(ctx context.Context, repo, remote string, refSpecs ...string) error {
	return nil
}

func (d *dryRunClient) LSRemote(ctx context.Context, url string) (string, error) {
	return d.real.LSRemote(ctx, url)
}
