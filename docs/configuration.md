# Configuration

`git-manager` reads a declarative TOML file that defines your repos, remotes, and per-repo git identity/signing setup. This guide covers the full schema and resolution rules.

## TOML schema

### Root: `[defaults]` and `[[groups]]`

```toml
[defaults]
# Optional identity/signing config applied to all repos that don't override it

user_name = "..."
user_email = "..."
signing_method = "..."  # "gpg", "ssh", or "none"
signing_key = "..."

[[groups]]
# A group organizes repos by path

name = "..."  # group name (required)
path = "..."  # directory path for repos in this group (required, supports ~)

# Optional identity/signing config at group level
user_name = "..."
user_email = "..."
signing_method = "..."
signing_key = "..."

  [[groups.repos]]
  # A repo in a group

  name = "..."  # repo name (required)

  # Optional identity/signing config at repo level
  user_name = "..."
  user_email = "..."
  signing_method = "..."
  signing_key = "..."

    [groups.repos.remotes.<name>]
    # A remote in a repo

    url = "..."  # Git URL (required)
    branches = ""  # Optional: fetch filter (glob or regex)
```

## Field reference

### Identity/Signing fields (in `[defaults]`, `[[groups]]`, and `[[groups.repos]]`)

| Field | Type | Purpose |
|---|---|---|
| `user_name` | string | Git author name (`user.name`) |
| `user_email` | string | Git author email (`user.email`) |
| `signing_method` | string | Signing mode: `"gpg"`, `"ssh"`, or `"none"` |
| `signing_key` | string | Signing key ID or path (e.g., `ABCDEF1234567890` or `~/.ssh/key.pub`) |

All identity/signing fields are optional. A field that is not declared at any level (defaults, group, repo) means: do not touch that git config key for that repo.

### Remote config

| Field | Type | Purpose |
|---|---|---|
| `url` | string | Git clone/fetch URL (required) |
| `branches` | string | Branch selection filter (optional). Empty = fetch all (default). |

The `branches` field supports glob patterns (e.g., `release/*`) or regexes (e.g., `^(main\|develop)$`). When set, only branches matching the pattern are fetched.

## Identity/Signing resolution: narrowest-wins

When `git-manager` applies config to a repo, it merges identity/signing setup across three levels:

1. **Defaults** (lowest priority) — `[defaults]` block
2. **Group** (middle priority) — `[[groups]]` block
3. **Repo** (highest priority) — `[[groups.repos]]` block

The narrowest (most specific) declaration wins. If a field is unset at all three levels, the corresponding git config key is left completely untouched.

**Example:**

```toml
[defaults]
user_email = "default@example.com"
signing_method = "gpg"

[[groups]]
name = "work"
user_email = "work@example.com"  # overrides default

  [[groups.repos]]
  name = "project-a"
  # user_email: inherits from group (work@example.com)
  # signing_method: inherits from defaults (gpg)
  # signing_key: not declared anywhere => git config untouched

  [[groups.repos]]
  name = "project-b"
  signing_method = "ssh"  # overrides group's inherited (gpg)
  # user_email: still inherits from group (work@example.com)
```

### Signing method mapping

The `signing_method` field maps directly to git config keys:

| `signing_method` value | Git config result |
|---|---|
| `"gpg"` | `gpg.format=openpgp` (GPG signing) |
| `"ssh"` | `gpg.format=ssh` (SSH key signing) |
| `"none"` | `commit.gpgsign=false` (no signing) |

The `signing_key` field maps to `user.signingkey` regardless of signing method.

## Complete example

Here is a worked configuration using placeholders:

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
signing_method = "ssh"
signing_key = "~/.ssh/work_signing.pub"

  [[groups.repos]]
  name = "example-project"
  # no identity/signing fields here => inherits the group's (ssh, work email)

    [groups.repos.remotes.origin]
    url = "git@github.com:example-org/example-project.git"
    [groups.repos.remotes.fork]
    url = "git@github.com:octocat/example-project.git"
    branches = "release/*"

  [[groups.repos]]
  name = "other-project"
  signing_method = "none"
  # repo overrides the group's signing_method; user_email still inherits from group

    [groups.repos.remotes.origin]
    url = "git@github.com:example-org/other-project.git"

[[groups]]
name = "personal"
path = "~/code/personal"
# no identity/signing fields at group level => falls through to defaults

  [[groups.repos]]
  name = "untouched-project"
  # no identity/signing fields anywhere but defaults

    [groups.repos.remotes.origin]
    url = "https://example.com/octocat/untouched-project.git"
```

## Validation

`git-manager` validates the config file at load time:

- **Unknown keys** are rejected with a clear error message. Typos in field names are caught immediately.
- **Invalid `signing_method` values** (anything other than `"gpg"`, `"ssh"`, or `"none"`) are rejected with a specific error.

Example error for an unknown key:

```
git-manager sync: config: unknown key(s): defaults.gpg_signing_key
```

Example error for an invalid `signing_method`:

```
config: groups[work].signing_method: invalid value "pgp" (must be gpg, ssh or none)
```

## Tips

1. **Use tilde expansion for paths**: `path = "~/code/work"` expands to your home directory.

2. **Private URLs for security**: Use SSH URLs (`git@github.com:...`) or local paths if possible; HTTPS `git@` is for auth via SSH agent.

3. **Branch selection**: Leave `branches` empty (or omit it) to fetch all branches. This is the default.

4. **Repo paths combine group + name**: If `group.path = "~/code/work"` and `repo.name = "example-project"`, the repo is cloned to `~/code/work/example-project`.

5. **Identity is per-repo**: Each repo gets its own local git config (`.git/config`). Changing the config file and re-running `sync` updates only that repo's local identity, never touching global or system git config.

## Next steps

- Run your first sync with [Usage](usage.md)
- Schedule regular syncs with [Installation](installation.md) (shell completions)
