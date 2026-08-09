// Package scheduler provides install/uninstall functionality for per-user
// systemd (Linux) and launchd (macOS) scheduling. Templates are embedded
// and substituted with actual paths before installation.
package scheduler

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Odilhao/git-manager/internal/platform"
)

// InstallOptions configures an install operation.
type InstallOptions struct {
	DryRun bool
}

// InstallResult describes what was (or would be) installed.
type InstallResult struct {
	Actions []string
}

// UninstallOptions configures an uninstall operation.
type UninstallOptions struct {
	DryRun bool
}

// UninstallResult describes what was (or would be) uninstalled.
type UninstallResult struct {
	Actions []string
}

// executor defines how to run subprocess commands. Used for testability.
type executor interface {
	Run(ctx context.Context, name string, args ...string) error
}

// realExecutor implements executor using os/exec.
type realExecutor struct{}

func (r *realExecutor) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Embedded template files.
//
//go:embed templates/systemd/* templates/launchd/*
var templateFS embed.FS

// Install installs the git-manager scheduler for the current user.
// On Linux, it installs systemd user units. On macOS, it installs a
// launchd LaunchAgent. On Windows, it returns a "not supported" message.
// The --dry-run mode reports what would be done without making any changes.
func Install(ctx context.Context, opts InstallOptions) (InstallResult, error) {
	return InstallWithExecutor(ctx, opts, &realExecutor{})
}

// InstallWithExecutor is the internal implementation, allowing tests
// to inject a mock executor.
func InstallWithExecutor(ctx context.Context, opts InstallOptions, exec executor) (InstallResult, error) {
	result := InstallResult{}

	switch runtime.GOOS {
	case "linux":
		return installLinux(ctx, opts, exec)
	case "darwin":
		return installDarwin(ctx, opts, exec)
	case "windows":
		result.Actions = append(result.Actions, "Windows is not yet supported; use 'git-manager install' on a Linux or macOS system")
		return result, nil
	default:
		return result, fmt.Errorf("scheduler: unsupported OS %q", runtime.GOOS)
	}
}

// Uninstall uninstalls the git-manager scheduler for the current user.
// On Linux, it disables and removes systemd user units. On macOS, it
// unloads and removes the launchd LaunchAgent. On Windows, it returns
// a "not supported" message.
func Uninstall(ctx context.Context, opts UninstallOptions) (UninstallResult, error) {
	return UninstallWithExecutor(ctx, opts, &realExecutor{})
}

// UninstallWithExecutor is the internal implementation, allowing tests
// to inject a mock executor.
func UninstallWithExecutor(ctx context.Context, opts UninstallOptions, exec executor) (UninstallResult, error) {
	result := UninstallResult{}

	switch runtime.GOOS {
	case "linux":
		return uninstallLinux(ctx, opts, exec)
	case "darwin":
		return uninstallDarwin(ctx, opts, exec)
	case "windows":
		result.Actions = append(result.Actions, "Windows is not yet supported; use 'git-manager uninstall' on a Linux or macOS system")
		return result, nil
	default:
		return result, fmt.Errorf("scheduler: unsupported OS %q", runtime.GOOS)
	}
}

// installLinux installs systemd user units.
func installLinux(ctx context.Context, opts InstallOptions, exec executor) (InstallResult, error) {
	result := InstallResult{}

	// Read and substitute templates first (this doesn't create directories)
	serviceContent, err := readAndSubstitute("templates/systemd/git-manager.service")
	if err != nil {
		return result, err
	}

	timerContent, err := readAndSubstitute("templates/systemd/git-manager.timer")
	if err != nil {
		return result, err
	}

	// For dry-run, we only need the path without creating it
	if opts.DryRun {
		// Use ConfigDirPath to get path without creating directories
		configDir, err := platform.ConfigDirPath()
		if err != nil {
			return result, fmt.Errorf("scheduler: config path: %w", err)
		}
		systemdDir := filepath.Join(configDir, "systemd", "user")

		result.Actions = append(result.Actions,
			fmt.Sprintf("would write %s", filepath.Join(systemdDir, "git-manager.service")),
			fmt.Sprintf("would write %s", filepath.Join(systemdDir, "git-manager.timer")),
			"would run: systemctl --user daemon-reload",
			"would run: systemctl --user enable git-manager.timer",
		)
		return result, nil
	}

	// Non-dry-run path: create directories and write files
	// Determine target directory - this creates the directory
	configDir, err := platform.ConfigDir()
	if err != nil {
		return result, fmt.Errorf("scheduler: config dir: %w", err)
	}

	systemdDir := filepath.Join(configDir, "systemd", "user")

	// Create log directory
	stateDir, err := platform.StateDir()
	if err != nil {
		return result, fmt.Errorf("scheduler: state dir: %w", err)
	}

	logDir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return result, fmt.Errorf("scheduler: create log dir: %w", err)
	}

	// Create systemd directory and write files
	if err := os.MkdirAll(systemdDir, 0o700); err != nil {
		return result, fmt.Errorf("scheduler: create systemd dir: %w", err)
	}

	serviceFile := filepath.Join(systemdDir, "git-manager.service")
	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0o644); err != nil {
		return result, fmt.Errorf("scheduler: write service file: %w", err)
	}
	result.Actions = append(result.Actions, fmt.Sprintf("wrote %s", serviceFile))

	timerFile := filepath.Join(systemdDir, "git-manager.timer")
	if err := os.WriteFile(timerFile, []byte(timerContent), 0o644); err != nil {
		return result, fmt.Errorf("scheduler: write timer file: %w", err)
	}
	result.Actions = append(result.Actions, fmt.Sprintf("wrote %s", timerFile))

	// Reload systemd
	if err := exec.Run(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return result, fmt.Errorf("scheduler: systemctl daemon-reload: %w", err)
	}
	result.Actions = append(result.Actions, "ran: systemctl --user daemon-reload")

	// Enable timer
	if err := exec.Run(ctx, "systemctl", "--user", "enable", "git-manager.timer"); err != nil {
		return result, fmt.Errorf("scheduler: systemctl enable: %w", err)
	}
	result.Actions = append(result.Actions, "ran: systemctl --user enable git-manager.timer")

	return result, nil
}

// installDarwin installs a launchd LaunchAgent.
func installDarwin(ctx context.Context, opts InstallOptions, exec executor) (InstallResult, error) {
	result := InstallResult{}

	// Read and substitute template first (this doesn't create directories)
	plistContent, err := readAndSubstitute("templates/launchd/com.example.git-manager.plist")
	if err != nil {
		return result, err
	}

	// Determine target directory
	home, err := os.UserHomeDir()
	if err != nil {
		return result, fmt.Errorf("scheduler: home dir: %w", err)
	}

	launchAgentsDir := filepath.Join(home, "Library", "LaunchAgents")
	plistFile := filepath.Join(launchAgentsDir, "com.example.git-manager.plist")

	// For dry-run, report what would be done without creating any directories
	if opts.DryRun {
		result.Actions = append(result.Actions,
			fmt.Sprintf("would write %s", plistFile),
			"would run: launchctl load "+plistFile,
		)
		return result, nil
	}

	// Non-dry-run path: create directories and write files
	if err := os.MkdirAll(launchAgentsDir, 0o700); err != nil {
		return result, fmt.Errorf("scheduler: create launchd dir: %w", err)
	}

	// Create log directory
	stateDir, err := platform.StateDir()
	if err != nil {
		return result, fmt.Errorf("scheduler: state dir: %w", err)
	}

	logDir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return result, fmt.Errorf("scheduler: create log dir: %w", err)
	}

	// Write plist file
	if err := os.WriteFile(plistFile, []byte(plistContent), 0o644); err != nil {
		return result, fmt.Errorf("scheduler: write plist file: %w", err)
	}
	result.Actions = append(result.Actions, fmt.Sprintf("wrote %s", plistFile))

	// Load LaunchAgent
	if err := exec.Run(ctx, "launchctl", "load", plistFile); err != nil {
		return result, fmt.Errorf("scheduler: launchctl load: %w", err)
	}
	result.Actions = append(result.Actions, fmt.Sprintf("ran: launchctl load %s", plistFile))

	return result, nil
}

// uninstallLinux removes systemd user units.
func uninstallLinux(ctx context.Context, opts UninstallOptions, exec executor) (UninstallResult, error) {
	result := UninstallResult{}

	// Use ConfigDirPath to get path without creating directories
	configDir, err := platform.ConfigDirPath()
	if err != nil {
		return result, fmt.Errorf("scheduler: config dir: %w", err)
	}

	systemdDir := filepath.Join(configDir, "systemd", "user")
	serviceFile := filepath.Join(systemdDir, "git-manager.service")
	timerFile := filepath.Join(systemdDir, "git-manager.timer")

	if opts.DryRun {
		result.Actions = append(result.Actions,
			"would run: systemctl --user disable git-manager.timer",
			"would run: systemctl --user stop git-manager.timer",
			fmt.Sprintf("would remove %s", serviceFile),
			fmt.Sprintf("would remove %s", timerFile),
		)
	} else {
		// Disable timer
		if err := exec.Run(ctx, "systemctl", "--user", "disable", "git-manager.timer"); err != nil {
			return result, fmt.Errorf("scheduler: systemctl disable: %w", err)
		}
		result.Actions = append(result.Actions, "ran: systemctl --user disable git-manager.timer")

		// Stop timer
		if err := exec.Run(ctx, "systemctl", "--user", "stop", "git-manager.timer"); err != nil {
			return result, fmt.Errorf("scheduler: systemctl stop: %w", err)
		}
		result.Actions = append(result.Actions, "ran: systemctl --user stop git-manager.timer")

		// Remove files
		if err := os.Remove(serviceFile); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("scheduler: remove service file: %w", err)
		}
		result.Actions = append(result.Actions, fmt.Sprintf("removed %s", serviceFile))

		if err := os.Remove(timerFile); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("scheduler: remove timer file: %w", err)
		}
		result.Actions = append(result.Actions, fmt.Sprintf("removed %s", timerFile))
	}

	return result, nil
}

// uninstallDarwin removes the launchd LaunchAgent.
func uninstallDarwin(ctx context.Context, opts UninstallOptions, exec executor) (UninstallResult, error) {
	result := UninstallResult{}

	home, err := os.UserHomeDir()
	if err != nil {
		return result, fmt.Errorf("scheduler: home dir: %w", err)
	}

	plistFile := filepath.Join(home, "Library", "LaunchAgents", "com.example.git-manager.plist")

	if opts.DryRun {
		result.Actions = append(result.Actions,
			"would run: launchctl unload "+plistFile,
			fmt.Sprintf("would remove %s", plistFile),
		)
	} else {
		// Unload LaunchAgent
		if err := exec.Run(ctx, "launchctl", "unload", plistFile); err != nil {
			return result, fmt.Errorf("scheduler: launchctl unload: %w", err)
		}
		result.Actions = append(result.Actions, fmt.Sprintf("ran: launchctl unload %s", plistFile))

		// Remove file
		if err := os.Remove(plistFile); err != nil && !os.IsNotExist(err) {
			return result, fmt.Errorf("scheduler: remove plist file: %w", err)
		}
		result.Actions = append(result.Actions, fmt.Sprintf("removed %s", plistFile))
	}

	return result, nil
}

// readAndSubstitute reads a template file and performs token substitution.
// This function does not create any directories on the filesystem.
func readAndSubstitute(templatePath string) (string, error) {
	content, err := templateFS.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("scheduler: read template %q: %w", templatePath, err)
	}

	text := string(content)

	// Substitute __GIT_MANAGER_BINARY__
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("scheduler: get executable path: %w", err)
	}
	text = strings.ReplaceAll(text, "__GIT_MANAGER_BINARY__", exe)

	// Substitute __CONFIG_PATH__
	// Use ConfigDirPath instead of ConfigDir to avoid creating directories
	configDir, err := platform.ConfigDirPath()
	if err != nil {
		return "", fmt.Errorf("scheduler: config path: %w", err)
	}
	configFile := filepath.Join(configDir, "config.toml")
	text = strings.ReplaceAll(text, "__CONFIG_PATH__", configFile)

	// Substitute __LOG_PATH__
	// Use StateDirPath instead of StateDir to avoid creating directories
	stateDir, err := platform.StateDirPath()
	if err != nil {
		return "", fmt.Errorf("scheduler: state path: %w", err)
	}
	logPath := filepath.Join(stateDir, "logs")
	text = strings.ReplaceAll(text, "__LOG_PATH__", logPath)

	// Substitute __USERNAME__
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("scheduler: get current user: %w", err)
	}
	text = strings.ReplaceAll(text, "__USERNAME__", currentUser.Username)

	return text, nil
}
