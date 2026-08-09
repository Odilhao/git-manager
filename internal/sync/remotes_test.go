package sync

import (
	"context"
	"sort"
	"testing"

	"github.com/Odilhao/git-manager/internal/gitcli"
)

// remoteRepo creates a real, ordinary (non-bare) git repo with no remotes,
// suitable as a target for ReconcileRemotes.
func remoteRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runFixture(t, dir, "init", "-b", "main")
	runFixture(t, dir, "config", "user.name", "Octocat")
	runFixture(t, dir, "config", "user.email", "octocat@example.com")
	return dir
}

func sortedNames(changes []RemoteChange) []string {
	names := make([]string, len(changes))
	for i, c := range changes {
		names[i] = c.Name
	}
	sort.Strings(names)
	return names
}

func TestReconcileRemotes_AddsMissingDeclaredRemote(t *testing.T) {
	repo := remoteRepo(t)
	c := gitcli.NewClient()
	declared := map[string]string{"origin": "https://example.com/example-org/example.git"}

	report, err := ReconcileRemotes(context.Background(), c, repo, declared, false)
	if err != nil {
		t.Fatalf("ReconcileRemotes: %v", err)
	}
	if len(report.Added) != 1 || report.Added[0] != (RemoteChange{Name: "origin", URL: declared["origin"]}) {
		t.Fatalf("report.Added = %+v, want one entry for origin", report.Added)
	}
	if len(report.Updated) != 0 || len(report.Removed) != 0 {
		t.Fatalf("report = %+v, want only Added populated", report)
	}

	current, err := c.RemoteList(context.Background(), repo)
	if err != nil {
		t.Fatalf("RemoteList: %v", err)
	}
	if current["origin"] != declared["origin"] {
		t.Fatalf("checkout remote origin = %q, want %q", current["origin"], declared["origin"])
	}
}

func TestReconcileRemotes_UpdatesChangedURL(t *testing.T) {
	repo := remoteRepo(t)
	runFixture(t, repo, "remote", "add", "origin", "https://example.com/example-org/old.git")
	c := gitcli.NewClient()
	declared := map[string]string{"origin": "https://example.com/example-org/new.git"}

	report, err := ReconcileRemotes(context.Background(), c, repo, declared, false)
	if err != nil {
		t.Fatalf("ReconcileRemotes: %v", err)
	}
	if len(report.Updated) != 1 || report.Updated[0] != (RemoteChange{Name: "origin", URL: declared["origin"]}) {
		t.Fatalf("report.Updated = %+v, want one entry for origin's new URL", report.Updated)
	}
	if len(report.Added) != 0 || len(report.Removed) != 0 {
		t.Fatalf("report = %+v, want only Updated populated", report)
	}

	current, err := c.RemoteList(context.Background(), repo)
	if err != nil {
		t.Fatalf("RemoteList: %v", err)
	}
	if current["origin"] != declared["origin"] {
		t.Fatalf("checkout remote origin = %q, want the updated URL %q", current["origin"], declared["origin"])
	}
}

func TestReconcileRemotes_LeavesUndeclaredRemoteAloneByDefault(t *testing.T) {
	repo := remoteRepo(t)
	runFixture(t, repo, "remote", "add", "origin", "https://example.com/example-org/example.git")
	runFixture(t, repo, "remote", "add", "scratch", "https://example.com/octocat/scratch.git")
	c := gitcli.NewClient()
	declared := map[string]string{"origin": "https://example.com/example-org/example.git"}

	report, err := ReconcileRemotes(context.Background(), c, repo, declared, false)
	if err != nil {
		t.Fatalf("ReconcileRemotes: %v", err)
	}
	if len(report.Added) != 0 || len(report.Updated) != 0 || len(report.Removed) != 0 {
		t.Fatalf("report = %+v, want no changes (declared remote already correct)", report)
	}

	current, err := c.RemoteList(context.Background(), repo)
	if err != nil {
		t.Fatalf("RemoteList: %v", err)
	}
	if url, ok := current["scratch"]; !ok || url != "https://example.com/octocat/scratch.git" {
		t.Fatalf("undeclared remote 'scratch' was touched: %q, present=%v", url, ok)
	}
}

func TestReconcileRemotes_RemovesUndeclaredRemoteWhenOverwrite(t *testing.T) {
	repo := remoteRepo(t)
	runFixture(t, repo, "remote", "add", "origin", "https://example.com/example-org/example.git")
	runFixture(t, repo, "remote", "add", "scratch", "https://example.com/octocat/scratch.git")
	c := gitcli.NewClient()
	declared := map[string]string{"origin": "https://example.com/example-org/example.git"}

	report, err := ReconcileRemotes(context.Background(), c, repo, declared, true)
	if err != nil {
		t.Fatalf("ReconcileRemotes: %v", err)
	}
	if len(report.Removed) != 1 || report.Removed[0].Name != "scratch" {
		t.Fatalf("report.Removed = %+v, want one entry for 'scratch'", report.Removed)
	}

	current, err := c.RemoteList(context.Background(), repo)
	if err != nil {
		t.Fatalf("RemoteList: %v", err)
	}
	if _, ok := current["scratch"]; ok {
		t.Fatalf("undeclared remote 'scratch' still present after overwrite=true: %v", current)
	}
}

// TestReconcileRemotes_OriginSurvivesOverwrite is the invariant-3 safety
// property: origin must never be deleted, even undeclared, even with
// overwrite=true — losing it would leave the checkout without its primary
// remote with no way back short of a fresh clone.
func TestReconcileRemotes_OriginSurvivesOverwrite(t *testing.T) {
	repo := remoteRepo(t)
	runFixture(t, repo, "remote", "add", "origin", "https://example.com/example-org/example.git")
	c := gitcli.NewClient()
	declared := map[string]string{} // origin is undeclared

	report, err := ReconcileRemotes(context.Background(), c, repo, declared, true)
	if err != nil {
		t.Fatalf("ReconcileRemotes: %v", err)
	}
	for _, removed := range report.Removed {
		if removed.Name == "origin" {
			t.Fatalf("report claims origin was removed: %+v", report.Removed)
		}
	}

	current, err := c.RemoteList(context.Background(), repo)
	if err != nil {
		t.Fatalf("RemoteList: %v", err)
	}
	if url, ok := current["origin"]; !ok || url != "https://example.com/example-org/example.git" {
		t.Fatalf("origin was deleted despite the always-survives rule: present=%v url=%q", ok, url)
	}
}

func TestReconcileRemotes_IdempotentSecondRunIsNoOp(t *testing.T) {
	repo := remoteRepo(t)
	c := gitcli.NewClient()
	declared := map[string]string{
		"origin": "https://example.com/example-org/example.git",
		"fork":   "https://example.com/octocat/example.git",
	}

	if _, err := ReconcileRemotes(context.Background(), c, repo, declared, true); err != nil {
		t.Fatalf("first ReconcileRemotes: %v", err)
	}

	report, err := ReconcileRemotes(context.Background(), c, repo, declared, true)
	if err != nil {
		t.Fatalf("second ReconcileRemotes: %v", err)
	}
	if len(report.Added) != 0 || len(report.Updated) != 0 || len(report.Removed) != 0 {
		t.Fatalf("second run report = %+v, want no changes", report)
	}
}

func TestReconcileRemotes_MultipleAddsAreSortedByName(t *testing.T) {
	repo := remoteRepo(t)
	c := gitcli.NewClient()
	declared := map[string]string{
		"origin": "https://example.com/example-org/example.git",
		"fork":   "https://example.com/octocat/example.git",
	}

	report, err := ReconcileRemotes(context.Background(), c, repo, declared, false)
	if err != nil {
		t.Fatalf("ReconcileRemotes: %v", err)
	}
	got := sortedNames(report.Added)
	want := []string{"fork", "origin"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("report.Added names = %v, want %v", got, want)
	}
}
