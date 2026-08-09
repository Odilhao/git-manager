// Package gitcli is a thin os/exec wrapper over the real git binary. It
// never reimplements git behaviour — every method shells out, so auth,
// signing and submodules behave exactly as they do for the user everywhere
// else.
package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Client runs git commands via the git binary on PATH.
type Client struct{}

// NewClient returns a Client that invokes the system git binary.
func NewClient() *Client {
	return &Client{}
}

// Error wraps a failed git invocation, preserving git's own exit code and
// stderr rather than a generic wrapped message.
type Error struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", e.ExitCode)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

func run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return "", &Error{Args: args, ExitCode: exitCode, Stderr: stderr.String()}
	}
	return stdout.String(), nil
}

var validScopes = map[string]bool{
	"local":  true,
	"global": true,
	"system": true,
}

func validateScope(scope string) error {
	if !validScopes[scope] {
		return fmt.Errorf("gitcli: invalid scope %q (must be local, global or system)", scope)
	}
	return nil
}

// Clone clones url into path. Unlike every other method, it does not operate
// on an existing checkout — path is where one is created.
func (c *Client) Clone(ctx context.Context, url, path string) error {
	_, err := run(ctx, "clone", url, path)
	return err
}

// RemoteAdd adds a remote named name pointing at url to repo.
func (c *Client) RemoteAdd(ctx context.Context, repo, name, url string) error {
	_, err := run(ctx, "-C", repo, "remote", "add", name, url)
	return err
}

// RemoteSetURL changes the URL of the remote named name in repo.
func (c *Client) RemoteSetURL(ctx context.Context, repo, name, url string) error {
	_, err := run(ctx, "-C", repo, "remote", "set-url", name, url)
	return err
}

// RemoteRemove deletes the remote named name from repo.
func (c *Client) RemoteRemove(ctx context.Context, repo, name string) error {
	_, err := run(ctx, "-C", repo, "remote", "remove", name)
	return err
}

// ConfigGet reads key from repo's config at scope ("local", "global" or
// "system").
func (c *Client) ConfigGet(ctx context.Context, repo, scope, key string) (string, error) {
	if err := validateScope(scope); err != nil {
		return "", err
	}
	out, err := run(ctx, "-C", repo, "config", "--"+scope, "--get", key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ConfigSet writes key=value to repo's config at scope ("local", "global" or
// "system").
func (c *Client) ConfigSet(ctx context.Context, repo, scope, key, value string) error {
	if err := validateScope(scope); err != nil {
		return err
	}
	_, err := run(ctx, "-C", repo, "config", "--"+scope, key, value)
	return err
}

// Fetch fetches from remote into repo, optionally with explicit refspecs.
func (c *Client) Fetch(ctx context.Context, repo, remote string, refSpecs ...string) error {
	args := append([]string{"-C", repo, "fetch", remote}, refSpecs...)
	_, err := run(ctx, args...)
	return err
}

// LSRemote lists refs on the remote repository at url without cloning it.
func (c *Client) LSRemote(ctx context.Context, url string) (string, error) {
	return run(ctx, "ls-remote", url)
}

// RevParse resolves ref to a commit hash in repo.
func (c *Client) RevParse(ctx context.Context, repo, ref string) (string, error) {
	out, err := run(ctx, "-C", repo, "rev-parse", ref)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
