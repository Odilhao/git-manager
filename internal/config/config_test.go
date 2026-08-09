package config

import (
	"reflect"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestLoadGolden(t *testing.T) {
	cfg, err := Load("testdata/golden.toml")
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if got, want := len(cfg.Groups), 2; got != want {
		t.Fatalf("len(Groups) = %d, want %d", got, want)
	}

	work := cfg.Groups[0]
	if work.Name != "work" || work.Path != "~/code/work" {
		t.Fatalf("unexpected work group: %+v", work)
	}
	if len(work.Repos) != 2 {
		t.Fatalf("len(work.Repos) = %d, want 2", len(work.Repos))
	}

	example := work.Repos[0]
	if example.Name != "example-project" {
		t.Fatalf("unexpected repo name: %s", example.Name)
	}
	if len(example.Remotes) != 2 {
		t.Fatalf("len(example.Remotes) = %d, want 2", len(example.Remotes))
	}
	if got, want := example.Remotes["origin"].URL, "git@github.com:example-org/example-project.git"; got != want {
		t.Fatalf("origin URL = %q, want %q", got, want)
	}
	if got, want := example.Remotes["fork"].URL, "git@github.com:octocat/example-project.git"; got != want {
		t.Fatalf("fork URL = %q, want %q", got, want)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	_, err := Load("testdata/unknown_key.toml")
	if err == nil {
		t.Fatal("Load: expected error for unknown key, got nil")
	}
}

func TestLoadRejectsInvalidSigningMethod(t *testing.T) {
	_, err := Load("testdata/bad_signing_method.toml")
	if err == nil {
		t.Fatal("Load: expected error for invalid signing_method, got nil")
	}
}

func TestLoadRejectsInvalidGroupSigningMethod(t *testing.T) {
	_, err := Load("testdata/bad_signing_method_group.toml")
	if err == nil {
		t.Fatal("Load: expected error for invalid group signing_method, got nil")
	}
	want := `config: groups[work].signing_method: invalid value "pgp" (must be gpg, ssh or none)`
	if err.Error() != want {
		t.Fatalf("Load error = %q, want %q", err.Error(), want)
	}
}

func TestLoadRejectsInvalidRepoSigningMethod(t *testing.T) {
	_, err := Load("testdata/bad_signing_method_repo.toml")
	if err == nil {
		t.Fatal("Load: expected error for invalid repo signing_method, got nil")
	}
	want := `config: groups[work].repos[example-project].signing_method: invalid value "pgp" (must be gpg, ssh or none)`
	if err.Error() != want {
		t.Fatalf("Load error = %q, want %q", err.Error(), want)
	}
}

func TestResolveThreeLevelMerge(t *testing.T) {
	cfg, err := Load("testdata/golden.toml")
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	resolved, err := cfg.Resolve()
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if got, want := len(resolved), 3; got != want {
		t.Fatalf("len(resolved) = %d, want %d", got, want)
	}

	byName := make(map[string]ResolvedRepo, len(resolved))
	for _, r := range resolved {
		byName[r.Name] = r
	}

	// example-project: no repo-level identity => inherits group (ssh, work
	// email) and falls through to defaults for user_name.
	example, ok := byName["example-project"]
	if !ok {
		t.Fatalf("resolved repo %q not found", "example-project")
	}
	assertStrPtr(t, "example-project.UserName", example.Identity.UserName, "Octocat")
	assertStrPtr(t, "example-project.UserEmail", example.Identity.UserEmail, "octocat@work.example.com")
	assertStrPtr(t, "example-project.SigningMethod", example.Identity.SigningMethod, "ssh")
	assertStrPtr(t, "example-project.SigningKey", example.Identity.SigningKey, "~/.ssh/work_signing.pub")
	if got, want := example.Path, "~/code/work/example-project"; got != want {
		t.Fatalf("example-project.Path = %q, want %q", got, want)
	}
	wantRemotes := map[string]RemoteConfig{
		"origin": {URL: "git@github.com:example-org/example-project.git"},
		"fork":   {URL: "git@github.com:octocat/example-project.git"},
	}
	if !reflect.DeepEqual(example.Remotes, wantRemotes) {
		t.Fatalf("example-project.Remotes = %+v, want %+v", example.Remotes, wantRemotes)
	}

	// other-project: repo-level signing_method overrides the group's.
	other, ok := byName["other-project"]
	if !ok {
		t.Fatalf("resolved repo %q not found", "other-project")
	}
	assertStrPtr(t, "other-project.UserEmail", other.Identity.UserEmail, "octocat@work.example.com")
	assertStrPtr(t, "other-project.SigningMethod", other.Identity.SigningMethod, "none")
	assertStrPtr(t, "other-project.SigningKey", other.Identity.SigningKey, "~/.ssh/work_signing.pub")

	// untouched-project: no group-level identity at all => falls through to
	// defaults entirely.
	untouched, ok := byName["untouched-project"]
	if !ok {
		t.Fatalf("resolved repo %q not found", "untouched-project")
	}
	assertStrPtr(t, "untouched-project.UserName", untouched.Identity.UserName, "Octocat")
	assertStrPtr(t, "untouched-project.SigningMethod", untouched.Identity.SigningMethod, "gpg")
	if got, want := untouched.Path, "~/code/personal/untouched-project"; got != want {
		t.Fatalf("untouched-project.Path = %q, want %q", got, want)
	}
	wantUntouchedRemotes := map[string]RemoteConfig{
		"origin": {URL: "https://example.com/octocat/untouched-project.git"},
	}
	if !reflect.DeepEqual(untouched.Remotes, wantUntouchedRemotes) {
		t.Fatalf("untouched-project.Remotes = %+v, want %+v", untouched.Remotes, wantUntouchedRemotes)
	}
}

func TestResolveUnsetFieldStaysNil(t *testing.T) {
	cfg, err := Load("testdata/no_identity.toml")
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	resolved, err := cfg.Resolve()
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	if len(resolved) != 1 {
		t.Fatalf("len(resolved) = %d, want 1", len(resolved))
	}

	id := resolved[0].Identity
	if id.UserName != nil || id.UserEmail != nil || id.SigningMethod != nil || id.SigningKey != nil {
		t.Fatalf("expected all identity fields nil (never declared), got %+v", id)
	}
}

func assertStrPtr(t *testing.T, field string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want %q", field, want)
	}
	if *got != want {
		t.Fatalf("%s = %q, want %q", field, *got, want)
	}
}
