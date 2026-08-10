# Release Downloads

## Getting the Latest Binary

Release binaries are published on the [GitHub Releases page](https://github.com/Odilhao/git-manager/releases).

Available platforms:
- **Linux**: `git-manager-<version>-linux-amd64`, `git-manager-<version>-linux-arm64`
- **macOS**: `git-manager-<version>-darwin-amd64`, `git-manager-<version>-darwin-arm64`

Each platform has both:
- A standalone binary (e.g., `git-manager-0.0.1-linux-amd64`)
- A tarball containing the binary and LICENSE file (e.g., `git-manager-0.0.1-linux-amd64.tar.gz`)

## Verification

Each artifact is accompanied by a checksum file (e.g., `git-manager-0.0.1-linux-amd64.sha256`).

### Verify a Binary

1. Download the binary and its `.sha256` checksum file to the same directory
2. Verify the checksum:
   ```bash
   sha256sum -c git-manager-0.0.1-linux-amd64.sha256
   ```

### Verify a Tarball

1. Download the tarball and its `.tar.gz.sha256` checksum file to the same directory
2. Verify the checksum:
   ```bash
   sha256sum -c git-manager-0.0.1-linux-amd64.tar.gz.sha256
   ```

A successful verification will print:
```
git-manager-0.0.1-linux-amd64: OK
```

## Installation

### From Binary

1. Download the appropriate binary for your platform
2. Make it executable:
   ```bash
   chmod +x git-manager-0.0.1-linux-amd64
   ```
3. Move it to a location in your `$PATH` (e.g., `~/.local/bin/` or `/usr/local/bin`)

### From Tarball

1. Download and extract:
   ```bash
   tar -xzf git-manager-0.0.1-linux-amd64.tar.gz
   ```
2. Make the binary executable:
   ```bash
   chmod +x git-manager-0.0.1-linux-amd64
   ```
3. Move it to a location in your `$PATH`

## Building from Source

If you prefer to build from source:

```bash
git clone https://github.com/Odilhao/git-manager.git
cd git-manager
make build
```

The binary will be created as `./git-manager` in the current directory.
