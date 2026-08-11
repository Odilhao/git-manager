// Package config loads and validates the declarative git-manager TOML
// config: the defaults/groups/repos/remotes schema and the narrowest-wins
// identity/signing resolution described in the project's root documentation.
package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the root of a decoded config file.
type Config struct {
	Defaults IdentityConfig `toml:"defaults"`
	Groups   []GroupConfig  `toml:"groups"`
}

// IdentityConfig holds the identity/signing fields shared by the defaults,
// group and repo levels. A nil field was never declared at that level; an
// empty string is a declared, deliberate value.
type IdentityConfig struct {
	UserName      *string `toml:"user_name"`
	UserEmail     *string `toml:"user_email"`
	SigningMethod *string `toml:"signing_method"`
	SigningKey    *string `toml:"signing_key"`
}

// GroupConfig is a `[[groups]]` table.
type GroupConfig struct {
	Name     string `toml:"name"`
	Path     string `toml:"path"`
	ReposDir string `toml:"repos_dir"`
	IdentityConfig
	Repos []RepoConfig `toml:"repos"`
}

// RepoConfig is a `[[groups.repos]]` table.
type RepoConfig struct {
	Name string `toml:"name"`
	IdentityConfig
	Remotes map[string]RemoteConfig `toml:"remotes"`
}

// RemoteConfig is a `[groups.repos.remotes.<name>]` table.
type RemoteConfig struct {
	URL string `toml:"url"`
	// Branches selects which branches to fetch from this remote: "" fetches
	// everything (git fetch's own default), a glob (e.g. "release/*") or a
	// regex (e.g. "^(main|develop)$") filters it. See internal/sync's
	// FetchBranches for the exact glob/regex distinction and semantics.
	Branches string `toml:"branches"`
}

var validSigningMethods = map[string]bool{
	"gpg":  true,
	"ssh":  true,
	"none": true,
}

// Load decodes and validates the config file at path. It rejects unknown
// keys and invalid field values loudly, at load time. It also loads and
// merges groups from config.d/ and repos_dir fragments relative to the main
// config file's directory.
func Load(configPath string) (*Config, error) {
	var cfg Config
	meta, err := toml.DecodeFile(configPath, &cfg)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("config: unknown key(s): %s", strings.Join(keys, ", "))
	}

	// Load fragments from config.d/ if present
	configDir := filepath.Dir(configPath)
	configDPath := filepath.Join(configDir, "config.d")
	if err := cfg.loadConfigD(configDPath); err != nil {
		return nil, err
	}

	// Load repos from repos_dir for each group
	if err := cfg.loadReposDirForGroups(configDir); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if err := c.Defaults.validate("defaults"); err != nil {
		return err
	}
	for i, g := range c.Groups {
		if err := g.IdentityConfig.validate(fmt.Sprintf("groups[%s]", g.Name)); err != nil {
			return err
		}
		// Validate no duplicate repo names within the group
		if err := c.validateNoDuplicateRepos(i); err != nil {
			return err
		}
		for _, r := range g.Repos {
			if err := r.IdentityConfig.validate(fmt.Sprintf("groups[%s].repos[%s]", g.Name, r.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (id IdentityConfig) validate(path string) error {
	if id.SigningMethod == nil {
		return nil
	}
	if !validSigningMethods[*id.SigningMethod] {
		return fmt.Errorf("config: %s.signing_method: invalid value %q (must be gpg, ssh or none)", path, *id.SigningMethod)
	}
	return nil
}

// ResolvedRepo is the per-repo output internal/sync will consume: the
// declared remotes and the identity/signing config after narrowest-wins
// merge across defaults, group and repo.
type ResolvedRepo struct {
	Name     string
	Path     string
	Remotes  map[string]RemoteConfig
	Identity ResolvedIdentity
}

// ResolvedIdentity mirrors IdentityConfig after merge: a nil field means no
// level ever declared it, so it must not be touched.
type ResolvedIdentity struct {
	UserName      *string
	UserEmail     *string
	SigningMethod *string
	SigningKey    *string
}

// Resolve produces one ResolvedRepo per declared repo, merging identity
// narrowest-wins: repo overrides group overrides defaults. Path resolution
// (tilde expansion, absolute paths) is deliberately out of scope here.
func (c *Config) Resolve() ([]ResolvedRepo, error) {
	var out []ResolvedRepo
	for _, g := range c.Groups {
		for _, r := range g.Repos {
			out = append(out, ResolvedRepo{
				Name:     r.Name,
				Path:     path.Join(g.Path, r.Name),
				Remotes:  r.Remotes,
				Identity: mergeIdentity(c.Defaults, g.IdentityConfig, r.IdentityConfig),
			})
		}
	}
	return out, nil
}

func mergeIdentity(levels ...IdentityConfig) ResolvedIdentity {
	var out ResolvedIdentity
	for _, lvl := range levels {
		if lvl.UserName != nil {
			out.UserName = lvl.UserName
		}
		if lvl.UserEmail != nil {
			out.UserEmail = lvl.UserEmail
		}
		if lvl.SigningMethod != nil {
			out.SigningMethod = lvl.SigningMethod
		}
		if lvl.SigningKey != nil {
			out.SigningKey = lvl.SigningKey
		}
	}
	return out
}

// loadConfigD loads and merges groups from all .toml files in config.d/,
// sorted lexically for determinism.
func (c *Config) loadConfigD(configDPath string) error {
	entries, err := ioutil.ReadDir(configDPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // config.d/ is optional
		}
		return fmt.Errorf("config: error reading config.d/: %w", err)
	}

	// Collect .toml files and sort lexically
	var tomlFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".toml") {
			tomlFiles = append(tomlFiles, entry.Name())
		}
	}
	sort.Strings(tomlFiles)

	// Load each fragment and merge groups
	for _, filename := range tomlFiles {
		filePath := filepath.Join(configDPath, filename)
		if err := c.mergeConfigDFragment(filePath, filename); err != nil {
			return err
		}
	}

	return nil
}

// mergeConfigDFragment loads a config.d/ fragment and merges its groups.
func (c *Config) mergeConfigDFragment(filePath string, filename string) error {
	var frag Config
	meta, err := toml.DecodeFile(filePath, &frag)
	if err != nil {
		return fmt.Errorf("config: config.d/%s: %w", filename, err)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return fmt.Errorf("config: config.d/%s: unknown key(s): %s", filename, strings.Join(keys, ", "))
	}

	// Merge groups
	for _, fg := range frag.Groups {
		existing := false
		for i := range c.Groups {
			if c.Groups[i].Name == fg.Name {
				existing = true
				// Validate that identity fields are not redeclared
				if err := c.validateGroupIdentityConflict(&c.Groups[i], &fg, filename); err != nil {
					return err
				}
				// Merge repos into existing group
				c.Groups[i].Repos = append(c.Groups[i].Repos, fg.Repos...)
				break
			}
		}
		if !existing {
			// New group from fragment
			c.Groups = append(c.Groups, fg)
		}
	}

	return nil
}

// validateGroupIdentityConflict checks that a group in config.d/ doesn't
// redefine path or identity fields that were already declared. Any re-declaration,
// even to the same value, is an error.
func (c *Config) validateGroupIdentityConflict(existing *GroupConfig, fragment *GroupConfig, filename string) error {
	// Path can never be re-declared (even to same value)
	if fragment.Path != "" && existing.Path != "" {
		return fmt.Errorf("config: conflict in groups[%s]: path cannot be redeclared (main config has %q, config.d/%s has %q)",
			existing.Name, existing.Path, filename, fragment.Path)
	}

	// Identity fields can never be re-declared (even to same value)
	if fragment.UserName != nil && existing.UserName != nil {
		return fmt.Errorf("config: conflict in groups[%s]: user_name cannot be redeclared", existing.Name)
	}
	if fragment.UserEmail != nil && existing.UserEmail != nil {
		return fmt.Errorf("config: conflict in groups[%s]: user_email cannot be redeclared", existing.Name)
	}
	if fragment.SigningMethod != nil && existing.SigningMethod != nil {
		return fmt.Errorf("config: conflict in groups[%s]: signing_method cannot be redeclared", existing.Name)
	}
	if fragment.SigningKey != nil && existing.SigningKey != nil {
		return fmt.Errorf("config: conflict in groups[%s]: signing_key cannot be redeclared", existing.Name)
	}

	return nil
}

// loadReposDirForGroups loads and merges repos from repos_dir for each group.
func (c *Config) loadReposDirForGroups(configDir string) error {
	for i := range c.Groups {
		if c.Groups[i].ReposDir != "" {
			reposDirPath := filepath.Join(configDir, c.Groups[i].ReposDir)
			if err := c.loadReposFromDir(i, reposDirPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// loadReposFromDir loads all .toml files from a repos_dir and merges their
// repos into the group at index groupIdx.
func (c *Config) loadReposFromDir(groupIdx int, reposDirPath string) error {
	entries, err := ioutil.ReadDir(reposDirPath)
	if err != nil {
		return fmt.Errorf("config: error reading repos_dir for groups[%s]: %w", c.Groups[groupIdx].Name, err)
	}

	// Collect .toml files and sort lexically
	var tomlFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".toml") {
			tomlFiles = append(tomlFiles, entry.Name())
		}
	}
	sort.Strings(tomlFiles)

	// Load each fragment
	for _, filename := range tomlFiles {
		filePath := filepath.Join(reposDirPath, filename)
		if err := c.mergeReposFragment(groupIdx, filePath, filename); err != nil {
			return err
		}
	}

	return nil
}

// mergeReposFragment loads a repos fragment file and merges its repos into
// the group at groupIdx.
func (c *Config) mergeReposFragment(groupIdx int, filePath string, filename string) error {
	var frag struct {
		Repos []RepoConfig `toml:"repos"`
	}

	meta, err := toml.DecodeFile(filePath, &frag)
	if err != nil {
		return fmt.Errorf("config: groups[%s].repos_dir: %s: %w", c.Groups[groupIdx].Name, filename, err)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return fmt.Errorf("config: groups[%s].repos_dir: %s: unknown key(s): %s",
			c.Groups[groupIdx].Name, filename, strings.Join(keys, ", "))
	}

	c.Groups[groupIdx].Repos = append(c.Groups[groupIdx].Repos, frag.Repos...)

	return nil
}

// validateNoDuplicateRepos checks that no repo names are duplicated within
// a group (across inline, repos_dir, and config.d/ sources).
func (c *Config) validateNoDuplicateRepos(groupIdx int) error {
	seen := make(map[string]bool)
	for _, repo := range c.Groups[groupIdx].Repos {
		if seen[repo.Name] {
			return fmt.Errorf("config: duplicate repo name %q in groups[%s]",
				repo.Name, c.Groups[groupIdx].Name)
		}
		seen[repo.Name] = true
	}
	return nil
}
