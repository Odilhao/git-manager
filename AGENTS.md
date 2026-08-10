# AGENTS.md

Guidance for AI coding agents and assistants working in this repository. See
`README.md` for the project overview and `docs/` for user-facing
documentation; this file is for anyone (human or agent) about to change code.

## What this project is

`git-manager` is a credential-free Go CLI that syncs local git checkouts
against a declarative TOML config — remotes, branch selection, and per-repo
git identity/signing setup. Single static binary, cross-compiled per
`GOOS`/`GOARCH`. See `README.md` and `docs/configuration.md` for the full
picture.

## Build, test, lint

```bash
CGO_ENABLED=0 go build -o git-manager ./cmd/git-manager   # or: make build
gofmt -l .                                                  # must be empty
go vet ./...
go test ./... -race -count=1
```

Run all three checks before proposing any change. `-count=1` disables the
test cache — a cached pass is evidence about an earlier tree, not this one.

## Layout

```
cmd/git-manager       CLI entrypoint and subcommands (sync, status, add, install, uninstall)
internal/config        TOML load, defaults, validation, the repo/group/remote schema
internal/gitcli        thin os/exec wrapper over the real `git` binary
internal/sync          plan/apply engine: path resolution, remote reconciliation, identity/signing
internal/platform       cross-platform config/state directory resolution
internal/scheduler      systemd --user / launchd install-uninstall logic
internal/status         drift reporting (status --check)
templates/              systemd/launchd unit templates, shell completions
docs/                   installation, configuration, usage, JSON schema, release docs
```

## Invariants — breaking one is a defect regardless of what a task asks for

1. **No credential is ever stored or required.** This tool relies entirely on
   the user's own git/SSH-agent/GPG-agent/credential-helper setup. It never
   reads, writes, generates, or transmits a secret, key, or token.
2. **Sync never rewrites history or moves branch pointers.** It manages remote
   configuration and local git identity/signing config, and runs `fetch` —
   never `push`, `merge`, `rebase`, or `commit`.
3. **Remote reconciliation is additive by default.** An undeclared remote is
   left alone unless the user passes `--overwrite`/`--prune` explicitly.
   Silent deletion is never the default path.
4. **Config validation rejects unknown keys.** A typo in a config file fails
   loudly at load time.
5. **Every dependency is declared in `go.mod`/`go.sum`.** No vendoring
   requirement, no hermetic build claim.
6. **Tests never touch the network or the invoking user's real
   `~/.gitconfig` or real repos.** Use real local git repos in temp dirs with
   `file://` remotes.
7. **No system-wide installation.** systemd/launchd units are always per-user
   (`systemctl --user`, `~/.config/systemd/user/`,
   `~/Library/LaunchAgents/`). Never asks for root.
8. **Identity/signing config is always written at local (per-repo) git config
   scope** — `git config --local`, never `--global` or `--system`.
9. **No personal or identifying data anywhere** — no real username, email,
   SSH/GPG key fingerprint, hostname, or a real repository URL in code,
   config, templates, tests, or docs. Use placeholders only
   (`example-org`, `octocat`, `example.com`). The module path
   `github.com/Odilhao/git-manager` is the sole exception — it is the
   repository's identity, not personal data.

## Conventions

- Go, MIT licence. The root `LICENSE` file is the sole licence statement — no
  `SPDX-License-Identifier` header on source files.
- Comments explain *why*, never *what*, and stay lean (short single-line
  comments on declarations, no prose paragraphs) — most code needs none.
- **Conventional Commits**, small and single-purpose: `feat(sync): …`,
  `fix(config): …`, `ci: …`. Subject under ~72 chars, imperative, lower case,
  no trailing period. `Closes #N` trailer where applicable.
- **Test-first.** Write the failing test before the implementation.
- Config/TOML/template examples use placeholder values only — see invariant 9.

## Not covered here

This repository also uses an internal, gitignored agentic development loop
(multi-agent intake/implementation/QA workflow) for the maintainer's own use —
not needed to build, test, or contribute to the code itself, so it isn't
described here.
