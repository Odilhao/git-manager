package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TemplateFiles maps embedded template paths to their corresponding repo-root paths
var templateFiles = map[string]string{
	"templates/systemd/git-manager.service":           "../../templates/systemd/git-manager.service",
	"templates/systemd/git-manager.timer":             "../../templates/systemd/git-manager.timer",
	"templates/launchd/com.example.git-manager.plist": "../../templates/launchd/com.example.git-manager.plist",
}

// mockExecutor records executor calls instead of running subprocesses.
type mockExecutor struct {
	calls []executorCall
	err   error
}

type executorCall struct {
	name string
	args []string
}

func (m *mockExecutor) Run(ctx context.Context, name string, args ...string) error {
	m.calls = append(m.calls, executorCall{name, slices.Clone(args)})
	return m.err
}

func TestInstall_Linux_WritesFiles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only test")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "") // Reset to allow default ~/.config
	t.Setenv("XDG_STATE_HOME", "")  // Reset to allow default ~/.local/state

	ctx := context.Background()
	mock := &mockExecutor{}

	opts := InstallOptions{DryRun: false}
	result, err := InstallWithExecutor(ctx, opts, mock)

	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify service and timer files were written
	// ConfigDir() returns ~/.config/git-manager, so full path is ~/.config/git-manager/systemd/user/
	serviceFile := filepath.Join(tmpHome, ".config", "git-manager", "systemd", "user", "git-manager.service")
	timerFile := filepath.Join(tmpHome, ".config", "git-manager", "systemd", "user", "git-manager.timer")

	if _, err := os.Stat(serviceFile); err != nil {
		t.Fatalf("service file not written: %v", err)
	}
	if _, err := os.Stat(timerFile); err != nil {
		t.Fatalf("timer file not written: %v", err)
	}

	// Verify file contents contain substituted tokens
	serviceContent, _ := os.ReadFile(serviceFile)
	if strings.Contains(string(serviceContent), "__GIT_MANAGER_BINARY__") {
		t.Error("service file still has __GIT_MANAGER_BINARY__ token")
	}
	if strings.Contains(string(serviceContent), "__CONFIG_PATH__") {
		t.Error("service file still has __CONFIG_PATH__ token")
	}

	// Verify systemctl calls were made
	if len(mock.calls) != 2 {
		t.Fatalf("expected 2 systemctl calls, got %d", len(mock.calls))
	}

	if mock.calls[0].name != "systemctl" || !slices.Contains(mock.calls[0].args, "daemon-reload") {
		t.Error("first systemctl call should be daemon-reload")
	}
	if mock.calls[1].name != "systemctl" || !slices.Contains(mock.calls[1].args, "enable") {
		t.Error("second systemctl call should be enable")
	}

	// Verify Actions are populated
	if len(result.Actions) == 0 {
		t.Error("Actions should be populated")
	}
}

func TestInstall_DryRun_NoFileWrites(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	ctx := context.Background()
	mock := &mockExecutor{}

	opts := InstallOptions{DryRun: true}
	result, err := InstallWithExecutor(ctx, opts, mock)

	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify NO files were written
	serviceFile := filepath.Join(tmpHome, ".config", "git-manager", "systemd", "user", "git-manager.service")
	if _, err := os.Stat(serviceFile); err == nil {
		t.Error("service file should NOT be written in dry-run mode")
	}

	// Verify NO systemctl calls were made
	if len(mock.calls) != 0 {
		t.Fatalf("expected 0 executor calls in dry-run, got %d", len(mock.calls))
	}

	// Verify Actions describe what WOULD happen
	if len(result.Actions) == 0 {
		t.Error("Actions should describe dry-run plan")
	}

	for _, action := range result.Actions {
		if !strings.Contains(action, "would") {
			t.Errorf("dry-run action should contain 'would': %q", action)
		}
	}
}

func TestInstall_Darwin_WritesPlist(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin only test")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	ctx := context.Background()
	mock := &mockExecutor{}

	opts := InstallOptions{DryRun: false}
	result, err := InstallWithExecutor(ctx, opts, mock)

	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify plist file was written
	plistFile := filepath.Join(tmpHome, "Library", "LaunchAgents", "com.example.git-manager.plist")

	if _, err := os.Stat(plistFile); err != nil {
		t.Fatalf("plist file not written: %v", err)
	}

	// Verify file contents contain substituted tokens
	plistContent, _ := os.ReadFile(plistFile)
	if strings.Contains(string(plistContent), "__GIT_MANAGER_BINARY__") {
		t.Error("plist file still has __GIT_MANAGER_BINARY__ token")
	}
	if strings.Contains(string(plistContent), "__CONFIG_PATH__") {
		t.Error("plist file still has __CONFIG_PATH__ token")
	}
	if strings.Contains(string(plistContent), "__LOG_PATH__") {
		t.Error("plist file still has __LOG_PATH__ token")
	}
	if strings.Contains(string(plistContent), "__USERNAME__") {
		t.Error("plist file still has __USERNAME__ token")
	}

	// Verify launchctl call was made
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 launchctl call, got %d", len(mock.calls))
	}

	if mock.calls[0].name != "launchctl" || !slices.Contains(mock.calls[0].args, "load") {
		t.Error("launchctl call should be load")
	}

	// Verify Actions are populated
	if len(result.Actions) == 0 {
		t.Error("Actions should be populated")
	}
}

func TestInstall_Windows_NoError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only test")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	ctx := context.Background()

	opts := InstallOptions{DryRun: false}
	result, err := Install(ctx, opts)

	if err != nil {
		t.Fatalf("Install should not error on Windows, got: %v", err)
	}

	// Verify a clear message is returned
	if len(result.Actions) == 0 {
		t.Error("Actions should contain a message about Windows not being supported")
	}

	found := false
	for _, action := range result.Actions {
		if strings.Contains(strings.ToLower(action), "not") && strings.Contains(strings.ToLower(action), "support") {
			found = true
			break
		}
	}

	if !found {
		t.Error("Actions should indicate Windows is not yet supported")
	}
}

func TestUninstall_Linux_RemovesFiles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only test")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	ctx := context.Background()

	// First install files
	Install(ctx, InstallOptions{DryRun: false})

	// Verify files exist
	serviceFile := filepath.Join(tmpHome, ".config", "git-manager", "systemd", "user", "git-manager.service")
	if _, err := os.Stat(serviceFile); err != nil {
		t.Fatalf("setup: service file should exist after Install: %v", err)
	}

	// Now uninstall
	mockUninstall := &mockExecutor{}
	result, err := UninstallWithExecutor(ctx, UninstallOptions{DryRun: false}, mockUninstall)

	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	// Verify files were removed
	if _, err := os.Stat(serviceFile); err == nil {
		t.Error("service file should be removed after Uninstall")
	}

	// Verify systemctl disable/stop was called before removal
	if len(mockUninstall.calls) < 2 {
		t.Fatalf("expected at least 2 systemctl calls (disable, stop), got %d", len(mockUninstall.calls))
	}

	hasDisable := false
	hasStop := false
	for _, call := range mockUninstall.calls {
		if call.name == "systemctl" {
			if slices.Contains(call.args, "disable") {
				hasDisable = true
			}
			if slices.Contains(call.args, "stop") {
				hasStop = true
			}
		}
	}

	if !hasDisable {
		t.Error("systemctl disable should be called")
	}
	if !hasStop {
		t.Error("systemctl stop should be called")
	}

	// Verify Actions are populated
	if len(result.Actions) == 0 {
		t.Error("Actions should be populated")
	}
}

func TestUninstall_DryRun_NoFileWrites(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	ctx := context.Background()

	// First install files
	Install(ctx, InstallOptions{DryRun: false})

	// Get the path to verify it exists before dry-run
	serviceFile := filepath.Join(tmpHome, ".config", "git-manager", "systemd", "user", "git-manager.service")
	if _, err := os.Stat(serviceFile); err != nil {
		t.Fatalf("setup: service file should exist after Install: %v", err)
	}

	// Now uninstall with dry-run
	mock := &mockExecutor{}
	result, err := UninstallWithExecutor(ctx, UninstallOptions{DryRun: true}, mock)

	if err != nil {
		t.Fatalf("Uninstall dry-run failed: %v", err)
	}

	// Verify files were NOT removed
	if _, err := os.Stat(serviceFile); err != nil {
		t.Error("service file should still exist after dry-run uninstall")
	}

	// Verify NO systemctl calls were made
	if len(mock.calls) != 0 {
		t.Fatalf("expected 0 executor calls in dry-run, got %d", len(mock.calls))
	}

	// Verify Actions describe what WOULD happen
	if len(result.Actions) == 0 {
		t.Error("Actions should describe dry-run plan")
	}

	for _, action := range result.Actions {
		if !strings.Contains(action, "would") {
			t.Errorf("dry-run action should contain 'would': %q", action)
		}
	}
}

func TestTokenSubstitution_AllTokensReplaced(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	ctx := context.Background()

	// Determine the expected paths
	if runtime.GOOS == "linux" {
		Install(ctx, InstallOptions{DryRun: false})

		serviceFile := filepath.Join(tmpHome, ".config", "git-manager", "systemd", "user", "git-manager.service")
		serviceContent, _ := os.ReadFile(serviceFile)
		serviceStr := string(serviceContent)

		// All tokens should be replaced
		if strings.Contains(serviceStr, "__GIT_MANAGER_BINARY__") {
			t.Error("__GIT_MANAGER_BINARY__ token not replaced")
		}
		if strings.Contains(serviceStr, "__CONFIG_PATH__") {
			t.Error("__CONFIG_PATH__ token not replaced")
		}

		// Verify actual values are present
		exe, _ := os.Executable()
		if !strings.Contains(serviceStr, exe) {
			t.Errorf("service file doesn't contain binary path %q", exe)
		}

	} else if runtime.GOOS == "darwin" {
		Install(ctx, InstallOptions{DryRun: false})

		plistFile := filepath.Join(tmpHome, "Library", "LaunchAgents", "com.example.git-manager.plist")
		plistContent, _ := os.ReadFile(plistFile)
		plistStr := string(plistContent)

		// All tokens should be replaced
		if strings.Contains(plistStr, "__GIT_MANAGER_BINARY__") {
			t.Error("__GIT_MANAGER_BINARY__ token not replaced")
		}
		if strings.Contains(plistStr, "__CONFIG_PATH__") {
			t.Error("__CONFIG_PATH__ token not replaced")
		}
		if strings.Contains(plistStr, "__LOG_PATH__") {
			t.Error("__LOG_PATH__ token not replaced")
		}
		if strings.Contains(plistStr, "__USERNAME__") {
			t.Error("__USERNAME__ token not replaced")
		}
	}
}

func TestInstall_CreatesLogDirectory(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_STATE_HOME", "")

	ctx := context.Background()
	Install(ctx, InstallOptions{DryRun: false})

	// Verify log directory was created
	logDir := filepath.Join(tmpHome, ".local", "state", "git-manager", "logs")
	info, err := os.Stat(logDir)
	if err != nil {
		t.Fatalf("log directory not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("log directory is not a directory")
	}

	// Check permissions (0o700)
	if info.Mode().Perm() != 0o700 {
		t.Errorf("log directory permissions are %o, expected 0o700", info.Mode().Perm())
	}
}

func TestInstall_DryRun_NoLogDirectory(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_STATE_HOME", "")

	ctx := context.Background()
	Install(ctx, InstallOptions{DryRun: true})

	// Verify log directory was NOT created
	logDir := filepath.Join(tmpHome, ".local", "state", "git-manager", "logs")
	if _, err := os.Stat(logDir); err == nil {
		t.Error("log directory should not be created in dry-run mode")
	}
}

func TestInstall_DryRun_NoParentDirectoriesCreated(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	ctx := context.Background()
	Install(ctx, InstallOptions{DryRun: true})

	// Verify parent .config/git-manager and .local/state/git-manager directories were NOT created
	configGitMgrDir := filepath.Join(tmpHome, ".config", "git-manager")
	if _, err := os.Stat(configGitMgrDir); err == nil {
		t.Errorf("config/git-manager directory should not be created in dry-run mode, but %s exists", configGitMgrDir)
	}

	stateGitMgrDir := filepath.Join(tmpHome, ".local", "state", "git-manager")
	if _, err := os.Stat(stateGitMgrDir); err == nil {
		t.Errorf("state/git-manager directory should not be created in dry-run mode, but %s exists", stateGitMgrDir)
	}
}

func TestTemplateFilesDoNotDrift(t *testing.T) {
	// Verify that embedded template files match their counterparts in the repo root.
	// This guards against the embedded copies drifting from the source-of-truth templates.
	for embeddedPath, repoPath := range templateFiles {
		embeddedContent, err := templateFS.ReadFile(embeddedPath)
		if err != nil {
			t.Fatalf("failed to read embedded template %q: %v", embeddedPath, err)
		}

		repoContent, err := os.ReadFile(repoPath)
		if err != nil {
			t.Fatalf("failed to read repo template %q: %v", repoPath, err)
		}

		if string(embeddedContent) != string(repoContent) {
			t.Errorf("template drift detected: embedded %q differs from repo %q", embeddedPath, repoPath)
		}
	}
}

func TestInstall_IsIdempotent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only test")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	ctx := context.Background()

	// First install
	mock1 := &mockExecutor{}
	result1, err := InstallWithExecutor(ctx, InstallOptions{DryRun: false}, mock1)
	if err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	// Second install should not error and should not duplicate systemctl calls
	mock2 := &mockExecutor{}
	result2, err := InstallWithExecutor(ctx, InstallOptions{DryRun: false}, mock2)
	if err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	// Both installs should make the same systemctl calls (2 calls each: daemon-reload, enable)
	if len(mock1.calls) != len(mock2.calls) {
		t.Errorf("systemctl call counts differ: first=%d, second=%d; idempotency broken", len(mock1.calls), len(mock2.calls))
	}

	if len(result1.Actions) != len(result2.Actions) {
		t.Errorf("action counts differ: first=%d, second=%d; idempotency broken", len(result1.Actions), len(result2.Actions))
	}
}

func TestUninstall_DryRun_NoParentDirectoriesCreated(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux only test")
	}

	// Fresh temp HOME with no prior install, to catch the bug where
	// uninstallLinux called ConfigDir() unconditionally before the dry-run check
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	ctx := context.Background()
	mock := &mockExecutor{}

	// Call uninstall with dry-run against a fresh HOME (no prior install)
	result, err := UninstallWithExecutor(ctx, UninstallOptions{DryRun: true}, mock)

	if err != nil {
		t.Fatalf("Uninstall dry-run failed: %v", err)
	}

	// Verify parent .config/git-manager and .local/state/git-manager directories were NOT created
	configGitMgrDir := filepath.Join(tmpHome, ".config", "git-manager")
	if _, err := os.Stat(configGitMgrDir); err == nil {
		t.Errorf("config/git-manager directory should not be created in dry-run mode, but %s exists", configGitMgrDir)
	}

	stateGitMgrDir := filepath.Join(tmpHome, ".local", "state", "git-manager")
	if _, err := os.Stat(stateGitMgrDir); err == nil {
		t.Errorf("state/git-manager directory should not be created in dry-run mode, but %s exists", stateGitMgrDir)
	}

	// Verify NO systemctl calls were made
	if len(mock.calls) != 0 {
		t.Fatalf("expected 0 executor calls in dry-run, got %d", len(mock.calls))
	}

	// Verify Actions describe what WOULD happen
	if len(result.Actions) == 0 {
		t.Error("Actions should describe dry-run plan")
	}

	for _, action := range result.Actions {
		if !strings.Contains(action, "would") {
			t.Errorf("dry-run action should contain 'would': %q", action)
		}
	}
}
