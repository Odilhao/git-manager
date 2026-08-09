package platform

import (
	"os"
	"path/filepath"
	"testing"
)

// testEnvVars holds environment variables for testing path computation logic.
type testEnvVars map[string]string

// getEnv returns the value of an environment variable from testEnvVars, or an
// empty string if unset. This mimics the signature of os.Getenv.
func (e testEnvVars) getEnv(key string) string {
	if v, ok := e[key]; ok {
		return v
	}
	return ""
}

func TestConfigDirLinuxDefaultPaths(t *testing.T) {
	home := "/home/testuser"
	env := testEnvVars{}

	path, err := computeConfigPath("linux", home, env.getEnv)
	if err != nil {
		t.Fatalf("computeConfigPath: unexpected error: %v", err)
	}

	want := filepath.Join(home, ".config", "git-manager")
	if path != want {
		t.Fatalf("computeConfigPath (linux, no env) = %q, want %q", path, want)
	}
}

func TestConfigDirLinuxXDGConfigHome(t *testing.T) {
	home := "/home/testuser"
	env := testEnvVars{
		"XDG_CONFIG_HOME": "/custom/config",
	}

	path, err := computeConfigPath("linux", home, env.getEnv)
	if err != nil {
		t.Fatalf("computeConfigPath: unexpected error: %v", err)
	}

	want := filepath.Join("/custom/config", "git-manager")
	if path != want {
		t.Fatalf("computeConfigPath (linux, XDG_CONFIG_HOME) = %q, want %q", path, want)
	}
}

func TestStateDirLinuxDefaultPaths(t *testing.T) {
	home := "/home/testuser"
	env := testEnvVars{}

	path, err := computeStatePath("linux", home, env.getEnv)
	if err != nil {
		t.Fatalf("computeStatePath: unexpected error: %v", err)
	}

	want := filepath.Join(home, ".local", "state", "git-manager")
	if path != want {
		t.Fatalf("computeStatePath (linux, no env) = %q, want %q", path, want)
	}
}

func TestStateDirLinuxXDGStateHome(t *testing.T) {
	home := "/home/testuser"
	env := testEnvVars{
		"XDG_STATE_HOME": "/custom/state",
	}

	path, err := computeStatePath("linux", home, env.getEnv)
	if err != nil {
		t.Fatalf("computeStatePath: unexpected error: %v", err)
	}

	want := filepath.Join("/custom/state", "git-manager")
	if path != want {
		t.Fatalf("computeStatePath (linux, XDG_STATE_HOME) = %q, want %q", path, want)
	}
}

func TestConfigDirMacOS(t *testing.T) {
	home := "/Users/testuser"
	env := testEnvVars{}

	path, err := computeConfigPath("darwin", home, env.getEnv)
	if err != nil {
		t.Fatalf("computeConfigPath: unexpected error: %v", err)
	}

	want := filepath.Join(home, "Library", "Application Support", "git-manager")
	if path != want {
		t.Fatalf("computeConfigPath (darwin) = %q, want %q", path, want)
	}
}

func TestStateDirMacOS(t *testing.T) {
	home := "/Users/testuser"
	env := testEnvVars{}

	path, err := computeStatePath("darwin", home, env.getEnv)
	if err != nil {
		t.Fatalf("computeStatePath: unexpected error: %v", err)
	}

	want := filepath.Join(home, "Library", "Application Support", "git-manager")
	if path != want {
		t.Fatalf("computeStatePath (darwin) = %q, want %q", path, want)
	}
}

func TestConfigDirWindows(t *testing.T) {
	home := "C:\\Users\\testuser"
	env := testEnvVars{
		"AppData": "C:\\Users\\testuser\\AppData\\Roaming",
	}

	path, err := computeConfigPath("windows", home, env.getEnv)
	if err != nil {
		t.Fatalf("computeConfigPath: unexpected error: %v", err)
	}

	want := filepath.Join("C:\\Users\\testuser\\AppData\\Roaming", "git-manager")
	if path != want {
		t.Fatalf("computeConfigPath (windows) = %q, want %q", path, want)
	}
}

func TestStateDirWindows(t *testing.T) {
	home := "C:\\Users\\testuser"
	env := testEnvVars{
		"AppData": "C:\\Users\\testuser\\AppData\\Roaming",
	}

	path, err := computeStatePath("windows", home, env.getEnv)
	if err != nil {
		t.Fatalf("computeStatePath: unexpected error: %v", err)
	}

	want := filepath.Join("C:\\Users\\testuser\\AppData\\Roaming", "git-manager")
	if path != want {
		t.Fatalf("computeStatePath (windows) = %q, want %q", path, want)
	}
}

func TestConfigDirRealFS(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	path, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: unexpected error: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat(%q): %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", path)
	}

	// Verify permissions are 0o700
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory permissions = %o, want 0o700", info.Mode().Perm())
	}

	// Verify it's in the expected location
	want := filepath.Join(tempDir, ".config", "git-manager")
	if path != want {
		t.Fatalf("ConfigDir = %q, want %q", path, want)
	}
}

func TestStateDirRealFS(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	path, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir: unexpected error: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat(%q): %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", path)
	}

	// Verify permissions are 0o700
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory permissions = %o, want 0o700", info.Mode().Perm())
	}

	// Verify it's in the expected location
	want := filepath.Join(tempDir, ".local", "state", "git-manager")
	if path != want {
		t.Fatalf("StateDir = %q, want %q", path, want)
	}
}

func TestConfigDirAlreadyExists(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	// Call ConfigDir first time to create the directory
	path1, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir (first): unexpected error: %v", err)
	}

	// Call ConfigDir again; it should return the same path without error
	path2, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir (second): unexpected error: %v", err)
	}

	if path1 != path2 {
		t.Fatalf("ConfigDir paths differ: %q vs %q", path1, path2)
	}
}

func TestStateDirAlreadyExists(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", "")

	// Call StateDir first time to create the directory
	path1, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir (first): unexpected error: %v", err)
	}

	// Call StateDir again; it should return the same path without error
	path2, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir (second): unexpected error: %v", err)
	}

	if path1 != path2 {
		t.Fatalf("StateDir paths differ: %q vs %q", path1, path2)
	}
}

func TestConfigDirErrorOnUnexpandableHome(t *testing.T) {
	// Simulate a case where home expansion fails by forcing a call to
	// computeConfigPath with an empty home. This tests error handling.
	_, err := computeConfigPath("linux", "", func(string) string { return "" })
	if err == nil {
		t.Fatal("computeConfigPath with empty home: expected error, got nil")
	}
}

func TestStateDirErrorOnUnexpandableHome(t *testing.T) {
	// Simulate a case where home expansion fails by forcing a call to
	// computeStatePath with an empty home. This tests error handling.
	_, err := computeStatePath("linux", "", func(string) string { return "" })
	if err == nil {
		t.Fatal("computeStatePath with empty home: expected error, got nil")
	}
}

// Test for Windows AppData not set in config-path computation.
func TestConfigDirWindowsAppDataMissing(t *testing.T) {
	home := "C:\\Users\\testuser"
	env := testEnvVars{} // AppData is unset

	_, err := computeConfigPath("windows", home, env.getEnv)
	if err == nil {
		t.Fatal("computeConfigPath (windows, no AppData): expected error, got nil")
	}
}

// Test for Windows AppData empty string in config-path computation.
func TestConfigDirWindowsAppDataEmpty(t *testing.T) {
	home := "C:\\Users\\testuser"
	env := testEnvVars{
		"AppData": "",
	}

	_, err := computeConfigPath("windows", home, env.getEnv)
	if err == nil {
		t.Fatal("computeConfigPath (windows, empty AppData): expected error, got nil")
	}
}

// Test for Windows AppData not set in state-path computation.
func TestStateDirWindowsAppDataMissing(t *testing.T) {
	home := "C:\\Users\\testuser"
	env := testEnvVars{} // AppData is unset

	_, err := computeStatePath("windows", home, env.getEnv)
	if err == nil {
		t.Fatal("computeStatePath (windows, no AppData): expected error, got nil")
	}
}

// Test for Windows AppData empty string in state-path computation.
func TestStateDirWindowsAppDataEmpty(t *testing.T) {
	home := "C:\\Users\\testuser"
	env := testEnvVars{
		"AppData": "",
	}

	_, err := computeStatePath("windows", home, env.getEnv)
	if err == nil {
		t.Fatal("computeStatePath (windows, empty AppData): expected error, got nil")
	}
}

// Test for unsupported GOOS in config-path computation.
func TestConfigDirUnsupportedOS(t *testing.T) {
	home := "/home/testuser"
	env := testEnvVars{}

	_, err := computeConfigPath("freebsd", home, env.getEnv)
	if err == nil {
		t.Fatal("computeConfigPath (freebsd): expected error, got nil")
	}
}

// Test for unsupported GOOS in state-path computation.
func TestStateDirUnsupportedOS(t *testing.T) {
	home := "/home/testuser"
	env := testEnvVars{}

	_, err := computeStatePath("freebsd", home, env.getEnv)
	if err == nil {
		t.Fatal("computeStatePath (freebsd): expected error, got nil")
	}
}

// Test ensureDir when path is a regular file, not a directory.
func TestEnsureDirFileExists(t *testing.T) {
	tempDir := t.TempDir()

	// Create a regular file at the path
	filePath := filepath.Join(tempDir, "testfile")
	if err := os.WriteFile(filePath, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// ensureDir should return an error when path exists but is not a directory
	_, err := ensureDir(filePath)
	if err == nil {
		t.Fatalf("ensureDir with existing file: expected error, got nil")
	}
}

// Test ConfigDir when path would be a regular file (integration test).
func TestConfigDirFileExists(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	// Pre-create the .config directory
	configPath := filepath.Join(tempDir, ".config")
	if err := os.MkdirAll(configPath, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create a regular file at git-manager path
	gitManagerPath := filepath.Join(configPath, "git-manager")
	if err := os.WriteFile(gitManagerPath, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// ConfigDir should return an error
	_, err := ConfigDir()
	if err == nil {
		t.Fatal("ConfigDir with existing file: expected error, got nil")
	}
}

// Test StateDir when path would be a regular file (integration test).
func TestStateDirFileExists(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", "")

	// Pre-create the .local/state directory
	statePath := filepath.Join(tempDir, ".local", "state")
	if err := os.MkdirAll(statePath, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Create a regular file at git-manager path
	gitManagerPath := filepath.Join(statePath, "git-manager")
	if err := os.WriteFile(gitManagerPath, []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// StateDir should return an error
	_, err := StateDir()
	if err == nil {
		t.Fatal("StateDir with existing file: expected error, got nil")
	}
}
