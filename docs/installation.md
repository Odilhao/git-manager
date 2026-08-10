# Installation

## Build from source

### Prerequisites

- Go 1.26 or later
- `git` (required for sync operations)

### Steps

1. Clone the repository:

```bash
git clone https://github.com/Odilhao/git-manager.git
cd git-manager
```

2. Build the binary:

```bash
make build
```

The resulting `git-manager` binary is placed in the current directory.

3. (Optional) Install to your `$PATH`:

```bash
install ./git-manager ~/.local/bin/
```

Or, if you prefer a system-wide location (requires write access):

```bash
sudo install ./git-manager /usr/local/bin/
```

## Prebuilt binaries

Download prebuilt release binaries for Linux and macOS from the [Releases](RELEASES.md) page. Binaries are provided for:

- Linux: x86_64, aarch64
- macOS: x86_64, aarch64 (Apple Silicon)

See [RELEASES.md](RELEASES.md) for download links and SHA256 checksum verification.

## Fedora / RHEL via COPR

The `git-manager` package is available in the official COPR repository for Fedora 44+:

```bash
dnf copr enable odilhao/git-manager
dnf install git-manager
```

To uninstall:

```bash
dnf remove git-manager
```

## Shell completions

Shell completion scripts are included for bash, zsh, and fish. See [Shell Completions](../templates/completions/README.md) for installation instructions.

## Platform notes

### Linux (primary target)

`git-manager` is fully tested and supported on modern Linux distributions (Fedora, Debian, Ubuntu, etc.). It respects XDG Base Directory specifications for config and state directories.

### macOS (primary target)

`git-manager` is fully tested and supported on macOS 11 (Big Sur) and later. It uses `~/Library/Application Support` for config and state directories.

### Windows

Windows support is explicitly deferred and not yet implemented. If you are on Windows, consider using Windows Subsystem for Linux (WSL) with the Linux build.

## Verification

After installation, verify the binary is executable and in your `$PATH`:

```bash
which git-manager
```

Or confirm it runs at all by invoking it with no arguments:

```bash
git-manager
```

You should see `usage: git-manager <command> [flags]` on stderr and a nonzero exit — that confirms the binary works. The available commands are `sync`, `status`, `add`, `install`, and `uninstall` (see the [usage guide](usage.md) for the full reference).

If `git-manager` is not in your `$PATH`, verify it works by using the full path to the binary:

```bash
./git-manager
```

## Next steps

Once installed, see [Configuration](configuration.md) to set up your first config file, or [Usage](usage.md) for the command reference.
