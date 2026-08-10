package status

import (
	"encoding/json"
	"testing"

	"github.com/Odilhao/git-manager/internal/sync"
)

func TestFromSyncReport_NoDriftWhenNothingChanged(t *testing.T) {
	sr := sync.Report{
		Repos: []sync.RepoResult{
			{Name: "example-project", Path: "/tmp/example-project"},
		},
	}

	got := FromSyncReport(sr)

	if got.Drifted {
		t.Fatalf("Drifted = true, want false for a repo report with no changes")
	}
	if len(got.Repos) != 1 || got.Repos[0].Drifted {
		t.Fatalf("Repos = %+v, want a single non-drifted repo", got.Repos)
	}
}

func TestFromSyncReport_ClonedMeansDrifted(t *testing.T) {
	sr := sync.Report{
		Repos: []sync.RepoResult{
			{Name: "example-project", Cloned: true},
		},
	}

	got := FromSyncReport(sr)

	if !got.Drifted {
		t.Fatalf("Drifted = false, want true when a repo would be cloned")
	}
	if !got.Repos[0].Drifted {
		t.Fatalf("Repos[0].Drifted = false, want true when Cloned is true")
	}
}

func TestFromSyncReport_RemoteChangeMeansDrifted(t *testing.T) {
	sr := sync.Report{
		Repos: []sync.RepoResult{
			{
				Name: "example-project",
				Remotes: sync.RemoteReport{
					Updated: []sync.RemoteChange{{Name: "origin", URL: "https://example.com/octocat/example.git"}},
				},
			},
		},
	}

	got := FromSyncReport(sr)

	if !got.Drifted || !got.Repos[0].Drifted {
		t.Fatalf("expected drift from a remote update, got %+v", got)
	}
	if len(got.Repos[0].Remotes.Updated) != 1 || got.Repos[0].Remotes.Updated[0].Name != "origin" {
		t.Fatalf("Remotes.Updated not carried through: %+v", got.Repos[0].Remotes)
	}
}

func TestFromSyncReport_IdentityWriteMeansDrifted(t *testing.T) {
	sr := sync.Report{
		Repos: []sync.RepoResult{
			{
				Name: "example-project",
				Identity: sync.IdentityReport{
					Written: []sync.IdentityWrite{{Key: "user.email", Value: "octocat@example.com"}},
				},
			},
		},
	}

	got := FromSyncReport(sr)

	if !got.Drifted || !got.Repos[0].Drifted {
		t.Fatalf("expected drift from an identity write, got %+v", got)
	}
}

func TestFromSyncReport_ErrorMeansDriftedAndCountsErrors(t *testing.T) {
	sr := sync.Report{
		ErrorCount: 1,
		Repos: []sync.RepoResult{
			{Name: "broken-project", Error: "sync: boom"},
		},
	}

	got := FromSyncReport(sr)

	if !got.Drifted || !got.Repos[0].Drifted {
		t.Fatalf("expected an errored repo to count as drifted, got %+v", got)
	}
	if got.ErrorCount != 1 {
		t.Fatalf("ErrorCount = %d, want 1", got.ErrorCount)
	}
	if got.Repos[0].Error != "sync: boom" {
		t.Fatalf("Error = %q, want the propagated error text", got.Repos[0].Error)
	}
}

// TestFromSyncReport_MirrorsOutcomeAndDurationMS proves status carries sync's
// derived Outcome and measured DurationMS through unchanged, rather than
// recomputing or dropping them.
func TestFromSyncReport_MirrorsOutcomeAndDurationMS(t *testing.T) {
	sr := sync.Report{
		DurationMS: 42,
		Repos: []sync.RepoResult{
			{Name: "example-project", Outcome: "success", DurationMS: 7},
		},
	}

	got := FromSyncReport(sr)

	if got.DurationMS != 42 {
		t.Fatalf("got.DurationMS = %d, want 42", got.DurationMS)
	}
	if got.Repos[0].Outcome != "success" {
		t.Fatalf("got.Repos[0].Outcome = %q, want %q", got.Repos[0].Outcome, "success")
	}
	if got.Repos[0].DurationMS != 7 {
		t.Fatalf("got.Repos[0].DurationMS = %d, want 7", got.Repos[0].DurationMS)
	}
}

// TestFromSyncReport_MirrorsNonSuccessOutcome proves the Outcome mirror
// isn't hardcoded to "success" — it must carry through "failure" (and, by
// the same code path, "partial") exactly as sync.Run computed it.
func TestFromSyncReport_MirrorsNonSuccessOutcome(t *testing.T) {
	sr := sync.Report{
		Repos: []sync.RepoResult{
			{Name: "no-origin", Outcome: "failure", Error: "sync: boom"},
		},
	}

	got := FromSyncReport(sr)

	if got.Repos[0].Outcome != "failure" {
		t.Fatalf("got.Repos[0].Outcome = %q, want %q", got.Repos[0].Outcome, "failure")
	}
}

// TestFromSyncReport_JSONFieldTypes asserts field types, not just presence —
// a status field silently marshaling as the wrong type would still pass a
// key-only check.
func TestFromSyncReport_JSONFieldTypes(t *testing.T) {
	sr := sync.Report{
		ErrorCount: 0,
		Repos: []sync.RepoResult{
			{
				Name: "example-project",
				Path: "/tmp/example-project",
				Remotes: sync.RemoteReport{
					Added: []sync.RemoteChange{{Name: "fork", URL: "https://example.com/octocat/example.git"}},
				},
				Identity: sync.IdentityReport{
					Written: []sync.IdentityWrite{{Key: "user.email", Value: "octocat@example.com"}},
				},
			},
		},
	}

	report := FromSyncReport(sr)
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if _, ok := decoded["drifted"].(bool); !ok {
		t.Fatalf("drifted field missing or not a bool: %v", decoded["drifted"])
	}
	if _, ok := decoded["error_count"].(float64); !ok {
		t.Fatalf("error_count field missing or not a number: %v", decoded["error_count"])
	}
	repos, ok := decoded["repos"].([]any)
	if !ok || len(repos) != 1 {
		t.Fatalf("repos field missing or not an array of length 1: %v", decoded["repos"])
	}
	repo, ok := repos[0].(map[string]any)
	if !ok {
		t.Fatalf("repos[0] is not an object: %v", repos[0])
	}
	if _, ok := repo["drifted"].(bool); !ok {
		t.Fatalf("repos[0].drifted missing or not a bool: %v", repo["drifted"])
	}
	if _, ok := repo["cloned"].(bool); !ok {
		t.Fatalf("repos[0].cloned missing or not a bool: %v", repo["cloned"])
	}
	remotes, ok := repo["remotes"].(map[string]any)
	if !ok {
		t.Fatalf("repos[0].remotes missing or not an object: %v", repo["remotes"])
	}
	added, ok := remotes["added"].([]any)
	if !ok || len(added) != 1 {
		t.Fatalf("repos[0].remotes.added missing or not an array of length 1: %v", remotes["added"])
	}
	addedEntry, ok := added[0].(map[string]any)
	if !ok {
		t.Fatalf("repos[0].remotes.added[0] is not an object: %v", added[0])
	}
	if name, ok := addedEntry["name"].(string); !ok || name != "fork" {
		t.Fatalf("repos[0].remotes.added[0].name = %v, want string \"fork\"", addedEntry["name"])
	}
	identity, ok := repo["identity"].(map[string]any)
	if !ok {
		t.Fatalf("repos[0].identity missing or not an object: %v", repo["identity"])
	}
	mismatched, ok := identity["mismatched"].([]any)
	if !ok || len(mismatched) != 1 {
		t.Fatalf("repos[0].identity.mismatched missing or not an array of length 1: %v", identity["mismatched"])
	}
	mismatchedEntry, ok := mismatched[0].(map[string]any)
	if !ok {
		t.Fatalf("repos[0].identity.mismatched[0] is not an object: %v", mismatched[0])
	}
	if key, ok := mismatchedEntry["key"].(string); !ok || key != "user.email" {
		t.Fatalf("repos[0].identity.mismatched[0].key = %v, want string \"user.email\"", mismatchedEntry["key"])
	}
	if value, ok := mismatchedEntry["value"].(string); !ok || value != "octocat@example.com" {
		t.Fatalf("repos[0].identity.mismatched[0].value = %v, want string \"octocat@example.com\"", mismatchedEntry["value"])
	}
}
