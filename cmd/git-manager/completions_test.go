package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestBashCompletionSyntax verifies the bash completion script has valid syntax.
// Skips if bash is not available on the system.
func TestBashCompletionSyntax(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("bash not found: %v", err)
	}

	scriptPath := filepath.Join("..", "..", "templates", "completions", "bash", "git-manager")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("bash completion script not found at %s: %v", scriptPath, err)
	}

	// bash -n checks syntax without executing
	cmd := exec.Command(bashPath, "-n", scriptPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("bash syntax check failed: %v", err)
	}
}

// TestZshCompletionSyntax verifies the zsh completion script has valid syntax.
// Skips if zsh is not available on the system.
func TestZshCompletionSyntax(t *testing.T) {
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skipf("zsh not found: %v", err)
	}

	scriptPath := filepath.Join("..", "..", "templates", "completions", "zsh", "_git-manager")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("zsh completion script not found at %s: %v", scriptPath, err)
	}

	// zsh -n checks syntax without executing
	cmd := exec.Command(zshPath, "-n", scriptPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("zsh syntax check failed: %v", err)
	}
}

// TestFishCompletionSyntax verifies the fish completion script has valid syntax.
// Skips if fish is not available on the system.
func TestFishCompletionSyntax(t *testing.T) {
	fishPath, err := exec.LookPath("fish")
	if err != nil {
		t.Skipf("fish not found: %v", err)
	}

	scriptPath := filepath.Join("..", "..", "templates", "completions", "fish", "git-manager.fish")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("fish completion script not found at %s: %v", scriptPath, err)
	}

	// fish --no-execute checks syntax without executing
	cmd := exec.Command(fishPath, "--no-execute", scriptPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("fish syntax check failed: %v", err)
	}
}
