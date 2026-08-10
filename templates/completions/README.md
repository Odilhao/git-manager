# Shell Completions

This directory contains static shell completion scripts for `git-manager`, supporting bash, zsh, and fish.

## Bash

Shell completion support for bash 3.2+.

### Installation

**Option 1: System-wide (if you have write access)**

Copy the completion script to the bash-completion directory:

```bash
sudo cp templates/completions/bash/git-manager \
    /usr/share/bash-completion/completions/git-manager
```

Then either restart your shell or source the completion directly:

```bash
source /usr/share/bash-completion/completions/git-manager
```

**Option 2: User-level (no sudo required)**

Copy to your local completion directory:

```bash
mkdir -p ~/.local/share/bash-completion/completions
cp templates/completions/bash/git-manager \
    ~/.local/share/bash-completion/completions/git-manager
```

Then add this to your `~/.bashrc`:

```bash
[[ -d ~/.local/share/bash-completion/completions ]] && \
    source ~/.local/share/bash-completion/completions/git-manager
```

**Option 3: Quick test (no installation)**

Source the script directly in your current shell:

```bash
source templates/completions/bash/git-manager
```

## Zsh

Shell completion support for zsh 4.3+.

### Installation

**Option 1: System-wide (if you have write access)**

Copy the completion script to the zsh completion directory:

```bash
sudo cp templates/completions/zsh/_git-manager \
    /usr/share/zsh/site-functions/_git-manager
```

Then restart your shell.

**Option 2: User-level (no sudo required)**

Create a completion directory and add it to your `fpath`:

```bash
mkdir -p ~/.zsh/completions
cp templates/completions/zsh/_git-manager \
    ~/.zsh/completions/_git-manager
```

Then add this to your `~/.zshrc` (before any `compinit` call):

```bash
fpath=(~/.zsh/completions $fpath)
autoload -U compinit && compinit
```

**Option 3: Quick test (no installation)**

Source the script directly:

```bash
mkdir -p ~/.zsh/completions
cp templates/completions/zsh/_git-manager ~/.zsh/completions/
fpath=(~/.zsh/completions $fpath)
autoload -U compinit && compinit
```

## Fish

Shell completion support for fish 3.0+.

### Installation

**Option 1: User-level (recommended)**

Copy the completion script to your fish completions directory:

```bash
mkdir -p ~/.config/fish/completions
cp templates/completions/fish/git-manager.fish \
    ~/.config/fish/completions/git-manager.fish
```

Completions are automatically loaded the next time you start a new fish shell.

**Option 2: System-wide (if you have write access)**

```bash
sudo cp templates/completions/fish/git-manager.fish \
    /usr/share/fish/vendor_completions.d/git-manager.fish
```

**Option 3: Quick test (no installation)**

Source the script directly in your current fish session:

```bash
source templates/completions/fish/git-manager.fish
```

## Testing Completions

After installation, test by typing the command and pressing Tab:

```bash
# Try subcommand completion
git-manager <TAB>

# Try flag completion for a subcommand
git-manager sync --<TAB>

# Try option values
git-manager sync --config /path/to/config<TAB>
```

## Supported Commands and Flags

All completion scripts cover:

**Subcommands:**
- `sync` — synchronize repos with configured state
- `status` — report repo status without making changes
- `add` — scaffold a config entry from existing checkout
- `install` — install scheduler (systemd/launchd)
- `uninstall` — uninstall scheduler (systemd/launchd)

**Global flags:**
- `--help` — show help message
- `--version` — show version

**Subcommand-specific flags:**

`sync` and `status`:
- `--config` — path to config file (required for both)
- `--dry-run` — report changes without applying (sync only)
- `--json` — output as JSON (both)
- `--parallel` — number of repos to process in parallel (both)
- `--overwrite` — remove undeclared remotes (sync only)
- `--prune` — remove undeclared remotes (sync only)

`add`:
- `--config` — path to config file (required)
- `--group` — group name (required)
- `--name` — repo name (required)
- `--dry-run` — print without writing

`install` and `uninstall`:
- `--dry-run` — report changes without applying

## Notes

- **Manual installation only.** These completion scripts are not installed by `git-manager install`; you must install them manually or via your package manager (if git-manager is packaged).
- **No credentials needed.** Completion scripts are purely text-based and do not invoke git or access any repositories.
- **Shell version requirements.** The scripts assume reasonably modern versions of bash (3.2+), zsh (4.3+), and fish (3.0+).
