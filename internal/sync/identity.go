package sync

import (
	"context"
	"fmt"

	"github.com/Odilhao/git-manager/internal/config"
)

// identityClient is the subset of *gitcli.Client ApplyIdentity depends on.
type identityClient interface {
	ConfigGet(ctx context.Context, repo, scope, key string) (string, error)
	ConfigSet(ctx context.Context, repo, scope, key, value string) error
}

// IdentityWrite records one git config key ApplyIdentity wrote.
type IdentityWrite struct {
	Key   string
	Value string
}

// IdentityReport records exactly what ApplyIdentity changed in a checkout.
type IdentityReport struct {
	Written []IdentityWrite
}

// ApplyIdentity writes identity's declared fields to repo's local git config
// only (git config --local, per the project's identity-scope invariant — a
// field is never applied at --global or --system). A nil field on identity
// was never declared at any config level and is left untouched: no read, no
// write, no key in the report. repoName and groupName are used only for error
// context wrapping; pass empty string for groupName if the repo is not in a group.
func ApplyIdentity(ctx context.Context, c identityClient, repo, repoName, groupName string, identity config.ResolvedIdentity) (IdentityReport, error) {
	var report IdentityReport

	if identity.UserName != nil {
		if err := setIfChanged(ctx, c, repo, "user.name", *identity.UserName, &report); err != nil {
			return report, err
		}
	}
	if identity.UserEmail != nil {
		if err := setIfChanged(ctx, c, repo, "user.email", *identity.UserEmail, &report); err != nil {
			return report, err
		}
	}
	if identity.SigningKey != nil {
		if err := setIfChanged(ctx, c, repo, "user.signingkey", *identity.SigningKey, &report); err != nil {
			return report, err
		}
	}
	if identity.SigningMethod != nil {
		key, value, err := signingMethodConfig(*identity.SigningMethod)
		if err != nil {
			// Wrap error with identifying context: repo name, path, and group when applicable.
			var contextStr string
			if groupName != "" {
				contextStr = fmt.Sprintf("repo %q in group %q (%s)", repoName, groupName, repo)
			} else {
				contextStr = fmt.Sprintf("repo %q (%s)", repoName, repo)
			}
			return report, fmt.Errorf("sync: %s: %w", contextStr, err)
		}
		if err := setIfChanged(ctx, c, repo, key, value, &report); err != nil {
			return report, err
		}
	}

	return report, nil
}

// signingMethodConfig maps a resolved signing_method to the single git
// config key/value it controls, per the binding mapping in CLAUDE.md.
func signingMethodConfig(method string) (key, value string, err error) {
	switch method {
	case "gpg":
		return "gpg.format", "openpgp", nil
	case "ssh":
		return "gpg.format", "ssh", nil
	case "none":
		return "commit.gpgsign", "false", nil
	default:
		return "", "", fmt.Errorf("sync: invalid signing_method %q", method)
	}
}

// setIfChanged writes key=value at local scope and records it, unless the
// checkout's local config already holds that exact value — this is what
// makes a second ApplyIdentity run report no changes.
func setIfChanged(ctx context.Context, c identityClient, repo, key, value string, report *IdentityReport) error {
	current, err := c.ConfigGet(ctx, repo, "local", key)
	if err == nil && current == value {
		return nil
	}
	if err := c.ConfigSet(ctx, repo, "local", key, value); err != nil {
		return fmt.Errorf("sync: set %s in %q: %w", key, repo, err)
	}
	report.Written = append(report.Written, IdentityWrite{Key: key, Value: value})
	return nil
}
