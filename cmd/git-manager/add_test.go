package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Odilhao/git-manager/internal/config"
)

// initRepoWithRemotesAndIdentity creates a real local git repo with two
// remotes and a local (never global) identity/signing config set, so `add`
// has real state to inspect and reproduce.
func initRepoWithRemotesAndIdentity(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "example-project")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runFixture(t, repo, "init", "-b", "main")
	runFixture(t, repo, "remote", "add", "origin", "https://example.com/example-org/example-project.git")
	runFixture(t, repo, "remote", "add", "fork", "https://example.com/octocat/example-project.git")
	runFixture(t, repo, "config", "--local", "user.name", "Octocat")
	runFixture(t, repo, "config", "--local", "user.email", "octocat@example.com")
	runFixture(t, repo, "config", "--local", "gpg.format", "ssh")
	runFixture(t, repo, "config", "--local", "user.signingkey", "~/.ssh/work_signing.pub")
	return repo
}

func writeEmptyConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("# existing config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func TestRunAdd_RequiresConfigGroupAndName(t *testing.T) {
	repo := initRepoWithRemotesAndIdentity(t)

	cases := [][]string{
		{"add", repo},
		{"add", "-config", writeEmptyConfig(t), repo},
		{"add", "-config", writeEmptyConfig(t), "-group", "work", repo},
		// Isolates the -config check specifically: -group and -name are
		// both present here, so a pass on this case can only come from the
		// "-config is required" branch firing on its own, not as a
		// fallback from some other missing flag.
		{"add", "-group", "work", "-name", "example-project", repo},
	}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		code := run(args, &stdout, &stderr)
		if code == 0 {
			t.Fatalf("run(%v) = 0, want non-zero (missing required flag)", args)
		}
		if stderr.Len() == 0 {
			t.Fatalf("run(%v): expected an error message on stderr", args)
		}
	}
}

// TestRunAdd_HelpFlagsPrintFlagUsageAndReturnZero checks -h/--help/help are
// handled before fs.Parse, so they never require -config/-group/-name and
// never collide with flag.ErrHelp's usual exit-2 path.
func TestRunAdd_HelpFlagsPrintFlagUsageAndReturnZero(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "help"} {
		t.Run(flag, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{"add", flag}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("run([add %q]) = %d, want 0; stderr: %s", flag, code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("run([add %q]) wrote to stderr, want stdout only: %s", flag, stderr.String())
			}
			if !strings.Contains(stdout.String(), "-group") {
				t.Fatalf("run([add %q]) usage missing flag -group:\n%s", flag, stdout.String())
			}
		})
	}
}

func TestRunAdd_NonRepoPathIsAnError(t *testing.T) {
	configPath := writeEmptyConfig(t)
	notARepo := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project", notARepo}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for a non-repo path")
	}
	// This asserts add's own explicit not-a-repo check fired (rather than
	// just happening to fail later for some other reason, e.g. a git
	// subcommand erroring against the same bad path).
	if !strings.Contains(stderr.String(), "is not a git repository") {
		t.Fatalf("expected add's own not-a-git-repository error, got: %s", stderr.String())
	}

	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# existing config\n" {
		t.Fatalf("config file was modified despite the error: %q", got)
	}
}

func TestRunAdd_WritesSchemaValidEntryReproducingRemotesAndIdentity(t *testing.T) {
	repo := initRepoWithRemotesAndIdentity(t)
	configPath := writeEmptyConfig(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project", repo}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("expected the generated snippet on stdout")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load after add: unexpected error: %v", err)
	}
	if len(cfg.Groups) != 1 {
		t.Fatalf("len(Groups) = %d, want 1", len(cfg.Groups))
	}
	if !cfg.RepoExists("work", "example-project") {
		t.Fatal("expected the new repo to be declared under group 'work'")
	}
	if cfg.Groups[0].Path != filepath.Dir(repo) {
		t.Fatalf("group.Path = %q, want %q (the checkout's parent directory)", cfg.Groups[0].Path, filepath.Dir(repo))
	}

	repoCfg := cfg.Groups[0].Repos[0]
	if repoCfg.Remotes["origin"].URL != "https://example.com/example-org/example-project.git" {
		t.Fatalf("origin URL = %q", repoCfg.Remotes["origin"].URL)
	}
	if repoCfg.Remotes["fork"].URL != "https://example.com/octocat/example-project.git" {
		t.Fatalf("fork URL = %q", repoCfg.Remotes["fork"].URL)
	}
	if repoCfg.UserName == nil || *repoCfg.UserName != "Octocat" {
		t.Fatalf("UserName = %v, want Octocat", repoCfg.UserName)
	}
	if repoCfg.UserEmail == nil || *repoCfg.UserEmail != "octocat@example.com" {
		t.Fatalf("UserEmail = %v, want octocat@example.com", repoCfg.UserEmail)
	}
	if repoCfg.SigningMethod == nil || *repoCfg.SigningMethod != "ssh" {
		t.Fatalf("SigningMethod = %v, want ssh", repoCfg.SigningMethod)
	}
	if repoCfg.SigningKey == nil || *repoCfg.SigningKey != "~/.ssh/work_signing.pub" {
		t.Fatalf("SigningKey = %v, want ~/.ssh/work_signing.pub", repoCfg.SigningKey)
	}
}

func TestRunAdd_SecondRunWithSameGroupAndNameErrorsAndLeavesFileUnchanged(t *testing.T) {
	repo := initRepoWithRemotesAndIdentity(t)
	configPath := writeEmptyConfig(t)

	var stdout1, stderr1 bytes.Buffer
	if code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project", repo}, &stdout1, &stderr1); code != 0 {
		t.Fatalf("first add: exit code = %d, stderr: %s", code, stderr1.String())
	}

	after1, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var stdout2, stderr2 bytes.Buffer
	code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project", repo}, &stdout2, &stderr2)
	if code == 0 {
		t.Fatal("second add with the same group+name: expected non-zero exit code, got 0")
	}
	if stderr2.Len() == 0 {
		t.Fatal("second add: expected an error message on stderr")
	}

	after2, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after1, after2) {
		t.Fatalf("config file changed after the rejected second add:\nbefore:\n%s\nafter:\n%s", after1, after2)
	}
}

func TestRunAdd_DryRunLeavesFileUntouchedAndPrintsSameSnippet(t *testing.T) {
	repo := initRepoWithRemotesAndIdentity(t)
	configPath := writeEmptyConfig(t)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var dryStdout, dryStderr bytes.Buffer
	code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project", "-dry-run", repo}, &dryStdout, &dryStderr)
	if code != 0 {
		t.Fatalf("dry-run exit code = %d, want 0; stderr: %s", code, dryStderr.String())
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("--dry-run modified the config file:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	afterInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if beforeInfo.ModTime() != afterInfo.ModTime() {
		t.Fatal("--dry-run touched the config file's mtime")
	}

	// A real (non-dry-run) run against a fresh, identical copy of the config
	// must print the exact same snippet dry-run did.
	realConfigPath := writeEmptyConfig(t)
	var realStdout, realStderr bytes.Buffer
	code = run([]string{"add", "-config", realConfigPath, "-group", "work", "-name", "example-project", repo}, &realStdout, &realStderr)
	if code != 0 {
		t.Fatalf("real run exit code = %d, want 0; stderr: %s", code, realStderr.String())
	}
	if dryStdout.String() != realStdout.String() {
		t.Fatalf("--dry-run snippet differs from the real run's snippet:\ndry-run:\n%s\nreal:\n%s", dryStdout.String(), realStdout.String())
	}
}

func TestRunAdd_DefaultsToCurrentWorkingDirectory(t *testing.T) {
	repo := initRepoWithRemotesAndIdentity(t)
	configPath := writeEmptyConfig(t)

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origWD) })
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RepoExists("work", "example-project") {
		t.Fatal("expected add without a positional path to use cwd")
	}
}

// Acceptance criterion 6: "Appends to the config file (or creates it if
// missing)." A -config path that doesn't exist yet must be created with the
// generated entry as its content, not treated as an error.
func TestRunAdd_CreatesConfigFileIfMissing(t *testing.T) {
	repo := initRepoWithRemotesAndIdentity(t)
	configPath := filepath.Join(t.TempDir(), "config.toml")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("precondition: %q must not exist yet", configPath)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project", repo}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("expected the generated snippet on stdout")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load(newly created config): unexpected error: %v", err)
	}
	if !cfg.RepoExists("work", "example-project") {
		t.Fatal("expected the new repo to be declared in the newly created config file")
	}
}

// The dry-run half of the same acceptance criterion: --dry-run must never
// create the file either, even when it's entirely missing.
func TestRunAdd_DryRunDoesNotCreateMissingConfigFile(t *testing.T) {
	repo := initRepoWithRemotesAndIdentity(t)
	configPath := filepath.Join(t.TempDir(), "config.toml")

	var stdout, stderr bytes.Buffer
	code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project", "-dry-run", repo}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("expected the generated snippet on stdout even though nothing was written")
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("--dry-run created %q, want it to remain absent", configPath)
	}
}

// no-global-scope guard: add must never read gpg.format/user.email/etc at
// global scope, even if the (isolated, temp) HOME's global gitconfig has
// values set that differ from local — only local-scope values may surface
// in the generated entry.
func TestRunAdd_NeverReadsGlobalScopeIdentity(t *testing.T) {
	repo := t.TempDir()
	runFixture(t, repo, "init", "-b", "main")
	runFixture(t, repo, "remote", "add", "origin", "https://example.com/example-org/example-project.git")
	// Deliberately no local identity/signing set at all. The isolated HOME's
	// global gitconfig (see TestMain) has none set either — this asserts
	// that if it did, add must still not surface any of the four fields, by
	// setting all of them globally and asserting every field reads nil.
	globalCfg := filepath.Join(os.Getenv("HOME"), ".gitconfig")
	globalContents := "[user]\n" +
		"\tname = Global Should Not Leak\n" +
		"\temail = global-should-not-leak@example.com\n" +
		"\tsigningkey = GLOBAL-KEY-SHOULD-NOT-LEAK\n" +
		"[gpg]\n" +
		"\tformat = openpgp\n"
	if err := os.WriteFile(globalCfg, []byte(globalContents), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(globalCfg) })

	configPath := writeEmptyConfig(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project", repo}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	repoCfg := cfg.Groups[0].Repos[0]
	if repoCfg.UserName != nil || repoCfg.UserEmail != nil || repoCfg.SigningMethod != nil || repoCfg.SigningKey != nil {
		t.Fatalf("expected all identity fields nil (global-scope values must never leak into a local-scope read), got %+v", repoCfg.IdentityConfig)
	}
}

func TestRunAdd_SigningMethodMapping(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(t *testing.T, repo string)
		wantMethod *string
	}{
		{
			name: "gpg",
			setup: func(t *testing.T, repo string) {
				runFixture(t, repo, "config", "--local", "gpg.format", "openpgp")
			},
			wantMethod: strPtrForTest("gpg"),
		},
		{
			name: "ssh",
			setup: func(t *testing.T, repo string) {
				runFixture(t, repo, "config", "--local", "gpg.format", "ssh")
			},
			wantMethod: strPtrForTest("ssh"),
		},
		{
			name: "none",
			setup: func(t *testing.T, repo string) {
				runFixture(t, repo, "config", "--local", "commit.gpgsign", "false")
			},
			wantMethod: strPtrForTest("none"),
		},
		{
			name: "none takes precedence over a stray gpg.format",
			setup: func(t *testing.T, repo string) {
				runFixture(t, repo, "config", "--local", "gpg.format", "openpgp")
				runFixture(t, repo, "config", "--local", "commit.gpgsign", "false")
			},
			wantMethod: strPtrForTest("none"),
		},
		{
			name:       "unset",
			setup:      func(t *testing.T, repo string) {},
			wantMethod: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			repo := filepath.Join(dir, "example-project")
			if err := os.MkdirAll(repo, 0o755); err != nil {
				t.Fatal(err)
			}
			runFixture(t, repo, "init", "-b", "main")
			runFixture(t, repo, "remote", "add", "origin", "https://example.com/example-org/example-project.git")
			tc.setup(t, repo)

			configPath := writeEmptyConfig(t)
			var stdout, stderr bytes.Buffer
			code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project", repo}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
			}

			cfg, err := config.Load(configPath)
			if err != nil {
				t.Fatal(err)
			}
			got := cfg.Groups[0].Repos[0].SigningMethod
			switch {
			case tc.wantMethod == nil && got != nil:
				t.Fatalf("SigningMethod = %q, want nil", *got)
			case tc.wantMethod != nil && (got == nil || *got != *tc.wantMethod):
				t.Fatalf("SigningMethod = %v, want %q", got, *tc.wantMethod)
			}
		})
	}
}

func TestRunAdd_NoLocalIdentitySetLeavesAllIdentityFieldsNil(t *testing.T) {
	repo := t.TempDir()
	runFixture(t, repo, "init", "-b", "main")
	runFixture(t, repo, "remote", "add", "origin", "https://example.com/example-org/example-project.git")
	configPath := writeEmptyConfig(t)

	var stdout, stderr bytes.Buffer
	code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project", repo}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	repoCfg := cfg.Groups[0].Repos[0]
	if repoCfg.UserName != nil || repoCfg.UserEmail != nil || repoCfg.SigningMethod != nil || repoCfg.SigningKey != nil {
		t.Fatalf("expected all identity fields nil, got %+v", repoCfg.IdentityConfig)
	}
}

// AddAgainstConfigWithExistingDefaultsTable guards against a real regression:
// if the appended fragment ever encoded a full config.Config (which carries
// a "defaults" field) rather than just the groups array, appending it to a
// config file that already declares its own non-empty [defaults] table
// would introduce a second [defaults] table — which BurntSushi's TOML
// decoder rejects as a duplicate key, breaking config.Load entirely.
func TestRunAdd_SurvivesConfigWithExistingDefaultsTable(t *testing.T) {
	repo := initRepoWithRemotesAndIdentity(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	contents := "[defaults]\n" +
		"user_email = \"octocat@example.com\"\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"add", "-config", configPath, "-group", "work", "-name", "example-project", repo}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, stderr.String())
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load after add: unexpected error (likely a duplicate [defaults] table): %v", err)
	}
	if cfg.Defaults.UserEmail == nil || *cfg.Defaults.UserEmail != "octocat@example.com" {
		t.Fatal("existing [defaults] table was lost or corrupted")
	}
	if !cfg.RepoExists("work", "example-project") {
		t.Fatal("expected the new repo to be declared under group 'work'")
	}
}

func strPtrForTest(s string) *string { return &s }
