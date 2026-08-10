package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

// Options controls how Run treats every repo it syncs.
type Options struct {
	// DryRun reports what would change without running any git command that
	// could mutate a checkout, a remote or its config.
	DryRun bool
	// Overwrite removes an undeclared remote during reconciliation (the
	// project's --overwrite/--prune flags both map to this one behavior).
	// origin is never removed, declared or not.
	Overwrite bool
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
// recorded on its RepoResult rather than aborting the rest.
func Run(ctx context.Context, real Client, cfg *config.Config, opts Options) Report {
	start := time.Now()
	resolved, _ := cfg.Resolve() // Resolve never actually returns a non-nil error today.

	report := Report{DryRun: opts.DryRun, Overwrite: opts.Overwrite}

	i := 0
	for _, g := range cfg.Groups {
		for _, r := range g.Repos {
			rr := resolved[i]
			i++
			result := syncRepo(ctx, real, g.Path, r.Name, rr, opts)
			if result.Error != "" {
				report.ErrorCount++
			}
			report.Repos = append(report.Repos, result)
		}
	}
	report.DurationMS = time.Since(start).Milliseconds()
	return report
}

func syncRepo(ctx context.Context, real Client, groupPath, repoName string, rr config.ResolvedRepo, opts Options) (result RepoResult) {
	start := time.Now()
	result = RepoResult{Name: repoName}
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

	identityReport, err := ApplyIdentity(ctx, client, path, rr.Identity)
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
