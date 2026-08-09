// Package status reshapes a sync.Report into a drift-oriented view for the
// read-only "status" command: same underlying data sync.Run already
// collected, framed as "what differs" rather than "what was applied".
package status

import "github.com/Odilhao/git-manager/internal/sync"

// RemoteChange mirrors sync.RemoteChange with stable JSON field names.
type RemoteChange struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// RemoteDrift reports the remote changes that a real sync would apply.
type RemoteDrift struct {
	Added   []RemoteChange `json:"added,omitempty"`
	Updated []RemoteChange `json:"updated,omitempty"`
	Removed []RemoteChange `json:"removed,omitempty"`
}

// IdentityMismatch mirrors sync.IdentityWrite with stable JSON field names.
type IdentityMismatch struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// IdentityDrift reports the identity/signing config keys that a real sync
// would write.
type IdentityDrift struct {
	Mismatched []IdentityMismatch `json:"mismatched,omitempty"`
}

// RepoResult is one repo's drift report.
type RepoResult struct {
	Name     string        `json:"name"`
	Path     string        `json:"path"`
	Drifted  bool          `json:"drifted"`
	Cloned   bool          `json:"cloned"`
	Remotes  RemoteDrift   `json:"remotes"`
	Identity IdentityDrift `json:"identity"`
	Error    string        `json:"error,omitempty"`
}

// Report is status's complete result: one RepoResult per repo declared in
// the config, in the same order sync.Report used.
type Report struct {
	Repos      []RepoResult `json:"repos"`
	Drifted    bool         `json:"drifted"`
	ErrorCount int          `json:"error_count"`
}

// FromSyncReport reshapes a sync.Report produced by sync.Run(..., DryRun:
// true) into a drift-oriented Report. It performs no comparison of its own —
// sync.Run's plan/apply functions, run against the dry-run client, already
// compute exactly this drift as a side effect of planning; this just renames
// "what would be applied" as "what has drifted".
func FromSyncReport(sr sync.Report) Report {
	report := Report{ErrorCount: sr.ErrorCount}
	for _, rr := range sr.Repos {
		repo := repoResultFrom(rr)
		if repo.Drifted {
			report.Drifted = true
		}
		report.Repos = append(report.Repos, repo)
	}
	return report
}

func repoResultFrom(rr sync.RepoResult) RepoResult {
	repo := RepoResult{
		Name:     rr.Name,
		Path:     rr.Path,
		Cloned:   rr.Cloned,
		Remotes:  remoteDriftFrom(rr.Remotes),
		Identity: identityDriftFrom(rr.Identity),
		Error:    rr.Error,
	}
	repo.Drifted = rr.Cloned ||
		len(repo.Remotes.Added) > 0 || len(repo.Remotes.Updated) > 0 || len(repo.Remotes.Removed) > 0 ||
		len(repo.Identity.Mismatched) > 0 ||
		rr.Error != ""
	return repo
}

func remoteDriftFrom(rr sync.RemoteReport) RemoteDrift {
	return RemoteDrift{
		Added:   remoteChangesFrom(rr.Added),
		Updated: remoteChangesFrom(rr.Updated),
		Removed: remoteChangesFrom(rr.Removed),
	}
}

func remoteChangesFrom(changes []sync.RemoteChange) []RemoteChange {
	if len(changes) == 0 {
		return nil
	}
	out := make([]RemoteChange, len(changes))
	for i, c := range changes {
		out[i] = RemoteChange{Name: c.Name, URL: c.URL}
	}
	return out
}

func identityDriftFrom(ir sync.IdentityReport) IdentityDrift {
	if len(ir.Written) == 0 {
		return IdentityDrift{}
	}
	out := make([]IdentityMismatch, len(ir.Written))
	for i, w := range ir.Written {
		out[i] = IdentityMismatch{Key: w.Key, Value: w.Value}
	}
	return IdentityDrift{Mismatched: out}
}
