package config

import (
	"reflect"
	"strings"
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
	if got, want := example.Remotes["fork"].Branches, "release/*"; got != want {
		t.Fatalf("fork Branches = %q, want %q", got, want)
	}
	if got := example.Remotes["origin"].Branches; got != "" {
		t.Fatalf("origin Branches = %q, want empty (unset means fetch everything)", got)
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
		"fork":   {URL: "git@github.com:octocat/example-project.git", Branches: "release/*"},
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

func TestLoadReposDir(t *testing.T) {
	cfg, err := Load("testdata/repos_dir_example/config.toml")
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if got, want := len(cfg.Groups), 1; got != want {
		t.Fatalf("len(Groups) = %d, want %d", got, want)
	}

	group := cfg.Groups[0]
	if group.Name != "example-org" {
		t.Fatalf("group name = %q, want %q", group.Name, "example-org")
	}

	// Should have inline repo + 3 repos from repos.d/ (service-a, service-b, service-c)
	if got, want := len(group.Repos), 4; got != want {
		t.Fatalf("len(group.Repos) = %d, want %d (inline + 3 from repos.d/)", got, want)
	}

	// Inline repo should be first
	if group.Repos[0].Name != "inline-repo" {
		t.Fatalf("first repo = %q, want %q", group.Repos[0].Name, "inline-repo")
	}

	// Repos from repos.d/ should follow in lexical order
	if group.Repos[1].Name != "service-a" {
		t.Fatalf("second repo = %q, want %q", group.Repos[1].Name, "service-a")
	}
	if group.Repos[2].Name != "service-b" {
		t.Fatalf("third repo = %q, want %q", group.Repos[2].Name, "service-b")
	}
	if group.Repos[3].Name != "service-c" {
		t.Fatalf("fourth repo = %q, want %q", group.Repos[3].Name, "service-c")
	}
}

func TestLoadConfigD(t *testing.T) {
	cfg, err := Load("testdata/config_d_example/config.toml")
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}

	if got, want := len(cfg.Groups), 2; got != want {
		t.Fatalf("len(Groups) = %d, want %d", got, want)
	}

	// Find work group
	var workGroup *GroupConfig
	var personalGroup *GroupConfig
	for i := range cfg.Groups {
		if cfg.Groups[i].Name == "work" {
			workGroup = &cfg.Groups[i]
		} else if cfg.Groups[i].Name == "personal" {
			personalGroup = &cfg.Groups[i]
		}
	}

	if workGroup == nil {
		t.Fatalf("work group not found")
	}
	if personalGroup == nil {
		t.Fatalf("personal group not found")
	}

	// Work group should have 2 repos: main-project from config, extra-project from config.d/
	if got, want := len(workGroup.Repos), 2; got != want {
		t.Fatalf("work group repos = %d, want %d", got, want)
	}

	// Personal group should have 1 repo
	if got, want := len(personalGroup.Repos), 1; got != want {
		t.Fatalf("personal group repos = %d, want %d", got, want)
	}
	if personalGroup.Repos[0].Name != "hobby-project" {
		t.Fatalf("personal repo name = %q, want %q", personalGroup.Repos[0].Name, "hobby-project")
	}
}

func TestLoadRejectsDuplicateRepoName(t *testing.T) {
	_, err := Load("testdata/duplicate_repo/config.toml")
	if err == nil {
		t.Fatal("Load: expected error for duplicate repo name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Load error = %q, want error containing 'duplicate'", err.Error())
	}
}

func TestLoadRejectsConflictingGroupIdentity(t *testing.T) {
	_, err := Load("testdata/conflict_identity/config.toml")
	if err == nil {
		t.Fatal("Load: expected error for conflicting group identity, got nil")
	}
	if !strings.Contains(err.Error(), "conflict") || !strings.Contains(err.Error(), "work") {
		t.Fatalf("Load error = %q, want error containing 'conflict' and 'work'", err.Error())
	}
}

func TestLoadRejectsSameValueGroupPathRedeclaration(t *testing.T) {
	// Even re-declaring the same path value in config.d/ should error
	_, err := Load("testdata/conflict_identity_same_value/config.toml")
	if err == nil {
		t.Fatal("Load: expected error for re-declared group path, got nil")
	}
	if !strings.Contains(err.Error(), "cannot be redeclared") {
		t.Fatalf("Load error = %q, want error containing 'cannot be redeclared'", err.Error())
	}
}

func TestLoadRejectsDuplicateRepoInlineAndConfigD(t *testing.T) {
	// Duplicate repo names across inline + config.d/ sources
	_, err := Load("testdata/duplicate_repo_config_d/config.toml")
	if err == nil {
		t.Fatal("Load: expected error for duplicate repo name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Load error = %q, want error containing 'duplicate'", err.Error())
	}
}

func TestLoadRejectsDuplicateRepoReposDirAndConfigD(t *testing.T) {
	// Duplicate repo names across repos_dir + config.d/ sources
	_, err := Load("testdata/duplicate_repo_repos_dir_config_d/config.toml")
	if err == nil {
		t.Fatal("Load: expected error for duplicate repo name, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Load error = %q, want error containing 'duplicate'", err.Error())
	}
}

func TestLoadRejectsUnknownKeyInReposFragment(t *testing.T) {
	_, err := Load("testdata/fragment_unknown_key/config.toml")
	if err == nil {
		t.Fatal("Load: expected error for unknown key in fragment, got nil")
	}
	if !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("Load error = %q, want error containing 'unknown key'", err.Error())
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
