# git-manager

A credential-free Go CLI that keeps local git checkouts in sync with a declarative TOML config — like a dotfiles manager, but for git repos.

## What it does

`git-manager` synchronizes your local git checkout's remotes, branch configuration, and per-repo identity/signing setup against a single declarative TOML file. Organize repos by group, declare their remotes and identity config once, and run `git-manager sync` (or schedule it via systemd/launchd) to bring your checkout into line — no credentials stored, no git history rewritten.

## Key features

- **Declarative config** — define repos, remotes, and per-repo identity/signing setup in a single TOML file
- **Multi-repo sync** — synchronize multiple repos (organized by groups) in one command
- **Per-repo identity/signing** — configure git identity, GPG/SSH signing per repo, per group, or globally (narrowest-wins resolution)
- **Scheduled runs** — built-in scheduler for systemd (Linux) and launchd (macOS) to keep repos in sync automatically
- **No credential storage** — relies entirely on your own git/SSH/GPG setup; `git-manager` never reads, writes, or transmits secrets
- **Single static binary** — cross-compiled for Linux, macOS; download or build in seconds

## Quick start

### 1. Install

**Option A: Build from source**

```bash
git clone https://github.com/Odilhao/git-manager.git
cd git-manager
make build
./git-manager sync -config ~/config.toml
```

**Option B: Fedora/RHEL via COPR**

```bash
dnf copr enable odilhao/git-manager
dnf install git-manager
git-manager sync -config ~/config.toml
```

### 2. Create a config file

Save this as `~/config.toml`:

```toml
[defaults]
user_name = "Octocat"
user_email = "octocat@example.com"
signing_method = "gpg"
signing_key = "ABCDEF1234567890"

[[groups]]
name = "work"
path = "~/code/work"
user_email = "octocat@work.example.com"

  [[groups.repos]]
  name = "example-project"

    [groups.repos.remotes.origin]
    url = "git@github.com:example-org/example-project.git"
    [groups.repos.remotes.fork]
    url = "git@github.com:octocat/example-project.git"
```

### 3. Run sync

```bash
git-manager sync -config ~/config.toml
```

This will clone the repo if missing, add/update/remove remotes to match the config, and apply identity/signing setup. Use `-dry-run` to preview changes before applying.

## Documentation

- **[Installation](docs/installation.md)** — build from source, prebuilt binaries, package manager, shell completions
- **[Configuration](docs/configuration.md)** — full TOML schema, identity/signing resolution, advanced examples
- **[Usage](docs/usage.md)** — command reference for `sync`, `status`, `add`, `install`, `uninstall`
- **[JSON output](docs/json-schema.md)** — machine-readable output shapes for `--json` flag
- **[Releases](docs/RELEASES.md)** — download prebuilt binaries, verify checksums, build from source

## License

MIT
