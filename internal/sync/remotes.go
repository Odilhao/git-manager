package sync

import (
	"context"
	"fmt"
	"sort"
)

// originRemoteName is carved out of the overwrite/prune path unconditionally
// — it is the checkout's primary remote, and deleting it (even undeclared)
// would leave the repo with no way back short of a fresh clone.
const originRemoteName = "origin"

// remoteClient is the subset of *gitcli.Client ReconcileRemotes depends on.
type remoteClient interface {
	RemoteList(ctx context.Context, repo string) (map[string]string, error)
	RemoteAdd(ctx context.Context, repo, name, url string) error
	RemoteSetURL(ctx context.Context, repo, name, url string) error
	RemoteRemove(ctx context.Context, repo, name string) error
}

// RemoteChange describes one remote that ReconcileRemotes added, updated or
// removed. For a removal, URL is the URL the remote had before deletion.
type RemoteChange struct {
	Name string
	URL  string
}

// RemoteReport records exactly what ReconcileRemotes changed in a checkout.
type RemoteReport struct {
	Added   []RemoteChange
	Updated []RemoteChange
	Removed []RemoteChange
}

// ReconcileRemotes brings repo's remotes into line with declared (name ->
// URL), additive by default per the project's remote-reconciliation
// invariant: a declared remote is added or corrected, but a remote already
// present that isn't declared is left alone unless overwrite is true. Even
// with overwrite true, "origin" is never removed, declared or not.
func ReconcileRemotes(ctx context.Context, c remoteClient, repo string, declared map[string]string, overwrite bool) (RemoteReport, error) {
	current, err := c.RemoteList(ctx, repo)
	if err != nil {
		return RemoteReport{}, fmt.Errorf("sync: list remotes in %q: %w", repo, err)
	}

	var report RemoteReport
	for _, name := range sortedKeys(declared) {
		url := declared[name]
		currentURL, exists := current[name]
		switch {
		case !exists:
			if err := c.RemoteAdd(ctx, repo, name, url); err != nil {
				return report, fmt.Errorf("sync: add remote %q in %q: %w", name, repo, err)
			}
			report.Added = append(report.Added, RemoteChange{Name: name, URL: url})
		case currentURL != url:
			if err := c.RemoteSetURL(ctx, repo, name, url); err != nil {
				return report, fmt.Errorf("sync: update remote %q in %q: %w", name, repo, err)
			}
			report.Updated = append(report.Updated, RemoteChange{Name: name, URL: url})
		}
	}

	if !overwrite {
		return report, nil
	}
	for _, name := range sortedKeys(current) {
		if name == originRemoteName {
			continue
		}
		if _, isDeclared := declared[name]; isDeclared {
			continue
		}
		if err := c.RemoteRemove(ctx, repo, name); err != nil {
			return report, fmt.Errorf("sync: remove remote %q in %q: %w", name, repo, err)
		}
		report.Removed = append(report.Removed, RemoteChange{Name: name, URL: current[name]})
	}

	return report, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
