// Package platform provides cross-platform config and state directory
// resolution. Paths are resolved per XDG Base Directory spec (Linux),
// ~/Library/Application Support (macOS), or %AppData% (Windows), with
// git-manager subdirectories created at 0o700 permissions as needed.
package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// envGetter is a function that retrieves environment variables. It exists
// to enable testing of all OS code paths on any platform.
type envGetter func(string) string

// ConfigDir returns the path to the git-manager config directory, creating
// it with 0o700 permissions if it doesn't exist. The path is determined by
// the platform: XDG_CONFIG_HOME/git-manager on Linux (or ~/.config/git-manager
// if XDG_CONFIG_HOME is unset), ~/Library/Application Support/git-manager on
// macOS, or %AppData%\git-manager on Windows.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("platform: config dir: get home: %w", err)
	}

	path, err := computeConfigPath(runtime.GOOS, home, os.Getenv)
	if err != nil {
		return "", err
	}

	return ensureDir(path)
}

// StateDir returns the path to the git-manager state directory, creating
// it with 0o700 permissions if it doesn't exist. The path is determined by
// the platform: XDG_STATE_HOME/git-manager on Linux (or ~/.local/state/git-manager
// if XDG_STATE_HOME is unset), ~/Library/Application Support/git-manager on
// macOS, or %AppData%\git-manager on Windows.
func StateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("platform: state dir: get home: %w", err)
	}

	path, err := computeStatePath(runtime.GOOS, home, os.Getenv)
	if err != nil {
		return "", err
	}

	return ensureDir(path)
}

// computeConfigPath computes the config directory path for a given OS,
// home directory, and environment variable getter. It does not create the
// directory or touch the filesystem.
func computeConfigPath(goos, home string, getenv envGetter) (string, error) {
	if home == "" {
		return "", fmt.Errorf("platform: config path: home directory required")
	}

	var base string

	switch goos {
	case "linux":
		xdgConfigHome := getenv("XDG_CONFIG_HOME")
		if xdgConfigHome != "" {
			base = xdgConfigHome
		} else {
			base = filepath.Join(home, ".config")
		}

	case "darwin":
		base = filepath.Join(home, "Library", "Application Support")

	case "windows":
		appData := getenv("AppData")
		if appData == "" {
			return "", fmt.Errorf("platform: config path: windows AppData not set")
		}
		base = appData

	default:
		return "", fmt.Errorf("platform: config path: unsupported OS %q", goos)
	}

	return filepath.Join(base, "git-manager"), nil
}

// computeStatePath computes the state directory path for a given OS,
// home directory, and environment variable getter. It does not create the
// directory or touch the filesystem.
func computeStatePath(goos, home string, getenv envGetter) (string, error) {
	if home == "" {
		return "", fmt.Errorf("platform: state path: home directory required")
	}

	var base string

	switch goos {
	case "linux":
		xdgStateHome := getenv("XDG_STATE_HOME")
		if xdgStateHome != "" {
			base = xdgStateHome
		} else {
			base = filepath.Join(home, ".local", "state")
		}

	case "darwin":
		base = filepath.Join(home, "Library", "Application Support")

	case "windows":
		appData := getenv("AppData")
		if appData == "" {
			return "", fmt.Errorf("platform: state path: windows AppData not set")
		}
		base = appData

	default:
		return "", fmt.Errorf("platform: state path: unsupported OS %q", goos)
	}

	return filepath.Join(base, "git-manager"), nil
}

// ensureDir creates the directory at path with 0o700 permissions if it
// doesn't exist, and returns the path. If the path exists and is already
// a directory, it returns the path without modifying it. Any other case
// (path is a file, permission denied, etc.) returns an error.
func ensureDir(path string) (string, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", fmt.Errorf("platform: create directory %q: %w", path, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("platform: stat directory %q: %w", path, err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("platform: %q exists but is not a directory", path)
	}

	return path, nil
}
