package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoExists(t *testing.T) {
	cfg := &Config{
		Groups: []GroupConfig{
			{
				Name: "work",
				Repos: []RepoConfig{
					{Name: "example-project"},
				},
			},
		},
	}

	if !cfg.RepoExists("work", "example-project") {
		t.Fatal("RepoExists: expected true for a declared repo, got false")
	}
	if cfg.RepoExists("work", "other-project") {
		t.Fatal("RepoExists: expected false for an undeclared repo name, got true")
	}
	if cfg.RepoExists("other-group", "example-project") {
		t.Fatal("RepoExists: expected false for an undeclared group, got true")
	}
}

func TestEncodeGroupFragmentRoundTripsThroughLoad(t *testing.T) {
	group := GroupConfig{
		Name: "work",
		Path: "/home/octocat/code/work",
		Repos: []RepoConfig{
			{
				Name: "example-project",
				IdentityConfig: IdentityConfig{
					UserEmail:     strPtr("octocat@example.com"),
					SigningMethod: strPtr("gpg"),
					SigningKey:    strPtr("ABCDEF1234567890"),
				},
				Remotes: map[string]RemoteConfig{
					"origin": {URL: "git@example.com:example-org/example-project.git"},
					"fork":   {URL: "git@example.com:octocat/example-project.git"},
				},
			},
		},
	}

	fragment, err := EncodeGroupFragment(group)
	if err != nil {
		t.Fatalf("EncodeGroupFragment: unexpected error: %v", err)
	}

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte(fragment), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load(EncodeGroupFragment output): unexpected error: %v\nfragment:\n%s", err, fragment)
	}
	if len(cfg.Groups) != 1 || len(cfg.Groups[0].Repos) != 1 {
		t.Fatalf("unexpected round-tripped shape: %+v", cfg.Groups)
	}
	repo := cfg.Groups[0].Repos[0]
	if repo.Name != "example-project" {
		t.Fatalf("repo.Name = %q, want %q", repo.Name, "example-project")
	}
	if repo.Remotes["origin"].URL != "git@example.com:example-org/example-project.git" {
		t.Fatalf("origin URL = %q", repo.Remotes["origin"].URL)
	}
	if repo.UserEmail == nil || *repo.UserEmail != "octocat@example.com" {
		t.Fatalf("UserEmail = %v, want octocat@example.com", repo.UserEmail)
	}
	if repo.SigningMethod == nil || *repo.SigningMethod != "gpg" {
		t.Fatalf("SigningMethod = %v, want gpg", repo.SigningMethod)
	}
}

func TestAppendGroupFragmentIsAppendOnly(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	original := "[[groups]]\n" +
		"name = \"other\"\n" +
		"path = \"/home/octocat/code/other\"\n\n" +
		"  # a comment that must survive\n" +
		"  [[groups.repos]]\n" +
		"  name = \"pre-existing\"\n"
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	group := GroupConfig{
		Name: "work",
		Path: "/home/octocat/code/work",
		Repos: []RepoConfig{
			{
				Name:    "example-project",
				Remotes: map[string]RemoteConfig{"origin": {URL: "git@example.com:example-org/example-project.git"}},
			},
		},
	}

	fragment, err := AppendGroupFragment(configPath, group)
	if err != nil {
		t.Fatalf("AppendGroupFragment: unexpected error: %v", err)
	}
	if fragment == "" {
		t.Fatal("AppendGroupFragment: expected non-empty fragment")
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), original) {
		t.Fatalf("existing file content was not preserved verbatim:\n%s", got)
	}
	if !strings.Contains(string(got), "a comment that must survive") {
		t.Fatal("existing comment was dropped")
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load after append: unexpected error: %v\ncontent:\n%s", err, got)
	}
	if len(cfg.Groups) != 2 {
		t.Fatalf("len(Groups) = %d, want 2 (existing + appended)", len(cfg.Groups))
	}
	if !cfg.RepoExists("other", "pre-existing") {
		t.Fatal("pre-existing repo was lost")
	}
	if !cfg.RepoExists("work", "example-project") {
		t.Fatal("appended repo was not written")
	}
}

func TestAppendGroupFragmentPreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[[groups]]\nname = \"other\"\npath = \"/tmp/other\"\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	group := GroupConfig{Name: "work", Path: "/home/octocat/code/work"}
	if _, err := AppendGroupFragment(configPath, group); err != nil {
		t.Fatalf("AppendGroupFragment: unexpected error: %v", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("file mode = %v, want 0640", info.Mode().Perm())
	}
}

func TestAppendGroupFragmentCreatesFileIfMissing(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("precondition: %q must not exist yet", configPath)
	}

	group := GroupConfig{
		Name: "work",
		Path: "/home/octocat/code/work",
		Repos: []RepoConfig{
			{
				Name:    "example-project",
				Remotes: map[string]RemoteConfig{"origin": {URL: "git@example.com:example-org/example-project.git"}},
			},
		},
	}

	fragment, err := AppendGroupFragment(configPath, group)
	if err != nil {
		t.Fatalf("AppendGroupFragment: unexpected error creating a missing config file: %v", err)
	}
	if fragment == "" {
		t.Fatal("AppendGroupFragment: expected a non-empty fragment")
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load(newly created config): unexpected error: %v", err)
	}
	if !cfg.RepoExists("work", "example-project") {
		t.Fatal("expected the new repo to be declared in the newly created config file")
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != fragment {
		t.Fatalf("newly created file content = %q, want exactly the fragment %q", got, fragment)
	}
}
