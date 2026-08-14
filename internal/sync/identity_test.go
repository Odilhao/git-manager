package sync

import (
	"context"
	"testing"

	"github.com/Odilhao/git-manager/internal/config"
	"github.com/Odilhao/git-manager/internal/gitcli"
)

func strPtr(s string) *string { return &s }

// identityRepo creates a real git repo with no local identity config at all,
// so ApplyIdentity's writes aren't masked by a fixture-set value.
func identityRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runFixture(t, dir, "init", "-b", "main")
	return dir
}

func TestApplyIdentity_UserNameAndEmail(t *testing.T) {
	repo := identityRepo(t)
	c := gitcli.NewClient()
	identity := config.ResolvedIdentity{
		UserName:  strPtr("Octocat"),
		UserEmail: strPtr("octocat@example.com"),
	}

	report, err := ApplyIdentity(context.Background(), c, repo, "example-project", "work", identity)
	if err != nil {
		t.Fatalf("ApplyIdentity: %v", err)
	}
	if len(report.Written) != 2 {
		t.Fatalf("report.Written = %+v, want 2 entries", report.Written)
	}

	name, err := c.ConfigGet(context.Background(), repo, "local", "user.name")
	if err != nil || name != "Octocat" {
		t.Fatalf("user.name = %q, err %v, want Octocat", name, err)
	}
	email, err := c.ConfigGet(context.Background(), repo, "local", "user.email")
	if err != nil || email != "octocat@example.com" {
		t.Fatalf("user.email = %q, err %v, want octocat@example.com", email, err)
	}
}

func TestApplyIdentity_SigningMethodGPG(t *testing.T) {
	repo := identityRepo(t)
	c := gitcli.NewClient()
	identity := config.ResolvedIdentity{SigningMethod: strPtr("gpg")}

	if _, err := ApplyIdentity(context.Background(), c, repo, "example-project", "work", identity); err != nil {
		t.Fatalf("ApplyIdentity: %v", err)
	}

	format, err := c.ConfigGet(context.Background(), repo, "local", "gpg.format")
	if err != nil || format != "openpgp" {
		t.Fatalf("gpg.format = %q, err %v, want openpgp", format, err)
	}
	if _, err := c.ConfigGet(context.Background(), repo, "local", "commit.gpgsign"); err == nil {
		t.Fatalf("commit.gpgsign should be unset for signing_method=gpg")
	}
}

func TestApplyIdentity_SigningMethodSSH(t *testing.T) {
	repo := identityRepo(t)
	c := gitcli.NewClient()
	identity := config.ResolvedIdentity{SigningMethod: strPtr("ssh")}

	if _, err := ApplyIdentity(context.Background(), c, repo, "example-project", "work", identity); err != nil {
		t.Fatalf("ApplyIdentity: %v", err)
	}

	format, err := c.ConfigGet(context.Background(), repo, "local", "gpg.format")
	if err != nil || format != "ssh" {
		t.Fatalf("gpg.format = %q, err %v, want ssh", format, err)
	}
}

func TestApplyIdentity_SigningMethodNone(t *testing.T) {
	repo := identityRepo(t)
	c := gitcli.NewClient()
	identity := config.ResolvedIdentity{SigningMethod: strPtr("none")}

	if _, err := ApplyIdentity(context.Background(), c, repo, "example-project", "work", identity); err != nil {
		t.Fatalf("ApplyIdentity: %v", err)
	}

	sign, err := c.ConfigGet(context.Background(), repo, "local", "commit.gpgsign")
	if err != nil || sign != "false" {
		t.Fatalf("commit.gpgsign = %q, err %v, want false", sign, err)
	}
	if _, err := c.ConfigGet(context.Background(), repo, "local", "gpg.format"); err == nil {
		t.Fatalf("gpg.format should be unset for signing_method=none")
	}
}

func TestApplyIdentity_SigningKey(t *testing.T) {
	repo := identityRepo(t)
	c := gitcli.NewClient()
	identity := config.ResolvedIdentity{SigningKey: strPtr("ABCDEF1234567890")}

	if _, err := ApplyIdentity(context.Background(), c, repo, "example-project", "work", identity); err != nil {
		t.Fatalf("ApplyIdentity: %v", err)
	}

	key, err := c.ConfigGet(context.Background(), repo, "local", "user.signingkey")
	if err != nil || key != "ABCDEF1234567890" {
		t.Fatalf("user.signingkey = %q, err %v, want ABCDEF1234567890", key, err)
	}
}

func TestApplyIdentity_FullyNilWritesNothing(t *testing.T) {
	repo := identityRepo(t)
	c := gitcli.NewClient()

	report, err := ApplyIdentity(context.Background(), c, repo, "example-project", "work", config.ResolvedIdentity{})
	if err != nil {
		t.Fatalf("ApplyIdentity: %v", err)
	}
	if len(report.Written) != 0 {
		t.Fatalf("report.Written = %+v, want none", report.Written)
	}

	for _, key := range []string{"user.name", "user.email", "user.signingkey", "gpg.format", "commit.gpgsign"} {
		if _, err := c.ConfigGet(context.Background(), repo, "local", key); err == nil {
			t.Fatalf("%s should be unset when identity is fully nil", key)
		}
	}
}

func TestApplyIdentity_PartiallyNilWritesOnlySetFields(t *testing.T) {
	repo := identityRepo(t)
	c := gitcli.NewClient()
	identity := config.ResolvedIdentity{UserEmail: strPtr("octocat@work.example.com")}

	report, err := ApplyIdentity(context.Background(), c, repo, "example-project", "work", identity)
	if err != nil {
		t.Fatalf("ApplyIdentity: %v", err)
	}
	if len(report.Written) != 1 || report.Written[0].Key != "user.email" {
		t.Fatalf("report.Written = %+v, want only user.email", report.Written)
	}

	email, err := c.ConfigGet(context.Background(), repo, "local", "user.email")
	if err != nil || email != "octocat@work.example.com" {
		t.Fatalf("user.email = %q, err %v, want octocat@work.example.com", email, err)
	}
	for _, key := range []string{"user.name", "user.signingkey", "gpg.format", "commit.gpgsign"} {
		if _, err := c.ConfigGet(context.Background(), repo, "local", key); err == nil {
			t.Fatalf("%s should remain unset when only user_email is declared", key)
		}
	}
}

func TestApplyIdentity_IdempotentSecondRunReportsNoChanges(t *testing.T) {
	repo := identityRepo(t)
	c := gitcli.NewClient()
	identity := config.ResolvedIdentity{
		UserName:      strPtr("Octocat"),
		UserEmail:     strPtr("octocat@example.com"),
		SigningMethod: strPtr("gpg"),
		SigningKey:    strPtr("ABCDEF1234567890"),
	}

	first, err := ApplyIdentity(context.Background(), c, repo, "example-project", "work", identity)
	if err != nil {
		t.Fatalf("first ApplyIdentity: %v", err)
	}
	if len(first.Written) != 4 {
		t.Fatalf("first report.Written = %+v, want 4 entries", first.Written)
	}

	second, err := ApplyIdentity(context.Background(), c, repo, "example-project", "work", identity)
	if err != nil {
		t.Fatalf("second ApplyIdentity: %v", err)
	}
	if len(second.Written) != 0 {
		t.Fatalf("second report.Written = %+v, want none (already correct)", second.Written)
	}

	// end state after two runs must still hold every value from the first
	wantAfter := map[string]string{
		"user.name":       "Octocat",
		"user.email":      "octocat@example.com",
		"user.signingkey": "ABCDEF1234567890",
		"gpg.format":      "openpgp",
	}
	for key, want := range wantAfter {
		got, err := c.ConfigGet(context.Background(), repo, "local", key)
		if err != nil || got != want {
			t.Fatalf("%s = %q, err %v, want %q after idempotent second run", key, got, err, want)
		}
	}
}
