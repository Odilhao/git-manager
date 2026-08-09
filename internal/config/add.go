package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// defaultConfigFileMode is used for a config file AppendGroupFragment
// creates from nothing. An existing file's own mode is always preserved
// instead — this is only the mode for a brand new one.
const defaultConfigFileMode = 0o644

// RepoExists reports whether a repo named repoName is already declared
// under a group named groupName anywhere in c. add uses this to refuse a
// silent duplicate before any write, rather than merging or overwriting.
func (c *Config) RepoExists(groupName, repoName string) bool {
	for _, g := range c.Groups {
		if g.Name != groupName {
			continue
		}
		for _, r := range g.Repos {
			if r.Name == repoName {
				return true
			}
		}
	}
	return false
}

// groupsFragment is what EncodeGroupFragment marshals: only a "groups"
// array, never "defaults" — so appending its output can never introduce a
// second [defaults] table into the target file.
type groupsFragment struct {
	Groups []GroupConfig `toml:"groups"`
}

// EncodeGroupFragment marshals a single self-contained [[groups]] block
// (one group, one repo) as TOML text. It never touches an existing config
// file; see AppendGroupFragment for that.
func EncodeGroupFragment(group GroupConfig) (string, error) {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(groupsFragment{Groups: []GroupConfig{group}}); err != nil {
		return "", fmt.Errorf("config: encode fragment: %w", err)
	}
	return buf.String(), nil
}

// AppendGroupFragment encodes group and appends it to the end of the config
// file at configPath, returning the appended text. If configPath doesn't
// exist yet, it is created with the fragment as its entire initial content.
// AppendGroupFragment never re-encodes or rewrites a file's existing bytes
// (which would strip comments/formatting) and it writes atomically via a
// temp file + rename, so a crash mid-write can never leave a truncated
// config on disk (or a half-created one).
func AppendGroupFragment(configPath string, group GroupConfig) (string, error) {
	fragment, err := EncodeGroupFragment(group)
	if err != nil {
		return "", err
	}

	mode := os.FileMode(defaultConfigFileMode)
	existing, err := os.ReadFile(configPath)
	switch {
	case err == nil:
		info, statErr := os.Stat(configPath)
		if statErr != nil {
			return "", fmt.Errorf("config: stat %q: %w", configPath, statErr)
		}
		mode = info.Mode().Perm()
	case errors.Is(err, os.ErrNotExist):
		existing = nil
	default:
		return "", fmt.Errorf("config: read %q: %w", configPath, err)
	}

	var out bytes.Buffer
	out.Write(existing)
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		out.WriteByte('\n')
	}
	if len(existing) > 0 {
		out.WriteByte('\n')
	}
	out.WriteString(fragment)

	tmp, err := os.CreateTemp(filepath.Dir(configPath), ".git-manager-config-*.tmp")
	if err != nil {
		return "", fmt.Errorf("config: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if _, err := tmp.Write(out.Bytes()); err != nil {
		tmp.Close()
		return "", fmt.Errorf("config: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("config: close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return "", fmt.Errorf("config: preserve file mode: %w", err)
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		return "", fmt.Errorf("config: rename temp file into place: %w", err)
	}

	return fragment, nil
}
