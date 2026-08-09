// Package config loads and validates the declarative git-manager TOML
// config: the defaults/groups/repos/remotes schema and the narrowest-wins
// identity/signing resolution described in the project's root documentation.
package config

import (
	"fmt"
	"path"
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
	Name string `toml:"name"`
	Path string `toml:"path"`
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
}

var validSigningMethods = map[string]bool{
	"gpg":  true,
	"ssh":  true,
	"none": true,
}

// Load decodes and validates the config file at path. It rejects unknown
// keys and invalid field values loudly, at load time.
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

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if err := c.Defaults.validate("defaults"); err != nil {
		return err
	}
	for _, g := range c.Groups {
		if err := g.IdentityConfig.validate(fmt.Sprintf("groups[%s]", g.Name)); err != nil {
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
