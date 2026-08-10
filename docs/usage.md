# Usage

## Commands

`git-manager` provides five main commands: `sync`, `status`, `add`, `install`, and `uninstall`.

### sync

Synchronize repos with declared config: clone if missing, reconcile remotes, apply identity/signing setup, and fetch from each remote's configured branches.

```bash
git-manager sync [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|---|---|---|---|
| `-config` | string | (required) | Path to the git-manager TOML config file |
| `-dry-run` | bool | false | Report what would change without applying it |
| `-overwrite` | bool | false | Remove undeclared remotes during reconciliation (alias: `--prune`) |
| `-prune` | bool | false | Remove undeclared remotes during reconciliation (alias: `--overwrite`) |
| `-json` | bool | false | Report as JSON instead of human-readable text |
| `-parallel` | int | 4 | Number of repos to sync in parallel (0 means default) |

**Examples:**

```bash
# Sync all repos with the default config
git-manager sync -config ~/.config/git-manager/config.toml

# Preview changes before applying (dry-run)
git-manager sync -config ~/.config/git-manager/config.toml -dry-run

# Sync and remove any undeclared remotes
git-manager sync -config ~/.config/git-manager/config.toml -overwrite

# Output results as JSON
git-manager sync -config ~/.config/git-manager/config.toml -json

# Sync with limited parallelism
git-manager sync -config ~/.config/git-manager/config.toml -parallel 2
```

**Output:** By default, `sync` prints one line per repo with status (OK or FAIL), path, operation summary, and duration. Use `-json` for machine-readable output (see [JSON Schema](json-schema.md)).

### status

Check for drift without making changes. Reports which repos are in sync, which have undeclared remotes, and which have identity/signing mismatches.

```bash
git-manager status [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|---|---|---|---|
| `-config` | string | (required) | Path to the git-manager TOML config file |
| `-json` | bool | false | Report as JSON instead of human-readable text |
| `-parallel` | int | 4 | Number of repos to check in parallel (0 means default) |

**Examples:**

```bash
# Check status of all repos
git-manager status -config ~/.config/git-manager/config.toml

# Output as JSON
git-manager status -config ~/.config/git-manager/config.toml -json
```

**Exit codes:**

- `0` — all repos are in sync
- `1` — one or more repos have drift
- `2` — command-line error (e.g., missing `-config`)

**Output:** Similar to `sync`, but labeled as "in sync", "drift", or "FAIL". Drift details include missing remotes, stale URLs, undeclared remotes, and identity mismatches.

### add

Scaffold a config entry from an existing local git checkout. Inspects the repo's current remotes and local git identity/signing config, and generates a TOML snippet ready to append to your config file.

```bash
git-manager add [flags] [repo-path]
```

**Flags:**

| Flag | Type | Default | Description |
|---|---|---|---|
| `-config` | string | (required) | Path to the git-manager TOML config file |
| `-group` | string | (required) | Group name for the new entry |
| `-name` | string | (required) | Repo name for the new entry |
| `-dry-run` | bool | false | Print the generated entry without writing it |

**Arguments:**

- `repo-path` (optional, default: `.`) — Path to the git repository. If omitted, uses the current directory.

**Examples:**

```bash
# Generate a config snippet for current repo, preview first
git-manager add -config ~/.config/git-manager/config.toml -group work -name my-project -dry-run

# Generate and append to config
git-manager add -config ~/.config/git-manager/config.toml -group work -name my-project

# Generate for a repo at a specific path
git-manager add -config ~/.config/git-manager/config.toml -group work -name my-project /path/to/repo
```

**Output:** Prints a TOML fragment with remotes and identity config detected from the repo. The fragment can be appended to your config file.

### install

Install the `git-manager` scheduler for automatic syncs. Creates per-user scheduler configuration (systemd timer on Linux, launchd agent on macOS).

```bash
git-manager install [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|---|---|---|---|
| `-dry-run` | bool | false | Report what would change without applying it |

**Examples:**

```bash
# Preview the installation
git-manager install -dry-run

# Install the scheduler
git-manager install
```

**What it does:**

- Creates the scheduler unit files in the appropriate per-user directory
- Does not require root or system-wide permissions
- Does not start the scheduler; you can do that manually with `systemctl --user start git-manager.timer` (Linux) or `launchctl start com.example.git-manager` (macOS)

See [Scheduling](../templates/README.md) for the manual setup walkthrough and customization options.

### uninstall

Remove the `git-manager` scheduler.

```bash
git-manager uninstall [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|---|---|---|---|
| `-dry-run` | bool | false | Report what would change without applying it |

**Examples:**

```bash
# Preview the uninstallation
git-manager uninstall -dry-run

# Uninstall the scheduler
git-manager uninstall
```

## Scheduling

The `install` and `uninstall` commands (added in issue #14) automate scheduler setup that was previously manual. See [Scheduling](../templates/README.md) for:

- Manual systemd timer configuration (Linux)
- Manual launchd agent configuration (macOS)
- Custom schedule times
- Viewing logs

The templates in `templates/systemd/` and `templates/launchd/` are also available for manual editing if you prefer more control over the setup.

## JSON output

The `-json` flag on `sync` and `status` outputs machine-readable JSON. The schema is defined in [JSON Schema](json-schema.md).

## Troubleshooting

### `git-manager sync: -config is required`

The `-config` flag must be supplied to `sync` and `status` subcommands. There is no default config path (yet); you must explicitly specify the path to your TOML config file.

**Solution:** Add `-config /path/to/config.toml` to the command, or set an environment variable in your shell profile:

```bash
export GIT_MANAGER_CONFIG=$HOME/.config/git-manager/config.toml
# Then in a script, you could use: git-manager sync -config "$GIT_MANAGER_CONFIG"
```

### `config: unknown key(s): ...`

The config file contains a typo or an unrecognized field name.

**Solution:** Check your TOML file against the schema in [Configuration](configuration.md). Common typos:
- `gpg_signing_key` (wrong) → `signing_key` (correct)
- `signing_method: gpg` (TOML value) → correct, but must be `"gpg"` (quoted string)
- Extra fields outside the schema

### `not a git repository`

The `add` command was given a path that is not a valid git repository.

**Solution:** Ensure the path contains a `.git` directory (or file, for worktrees) and is a valid git checkout.

### Remote URL mismatch warning

If a remote exists locally with a different URL than declared in the config, `sync` will update it by default (unless `-dry-run` is used).

**Solution:** If this is intentional, run `sync` normally to apply the update. If not, check your config file.

### Undeclared remotes preserved by default

`sync` leaves any remote not listed in the config untouched. To remove undeclared remotes, use `-overwrite` or `-prune`.

**Solution:** Run `git-manager sync -config ... -overwrite` to remove undeclared remotes. Or update your config to declare all intended remotes.

## Next steps

- [Configuration](configuration.md) — set up your config file
- [Installation](installation.md) — install `git-manager` and shell completions
