package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

// Config is the materialized config consumed by the CLI. All fields are
// defaulted during Load — callers never need to check for zero values.
type Config struct {
	TasksDir    string
	RepoRoots   []string
	TmuxPrefix  string
	DefaultBase string
	DefaultType string
	StartTmux   bool
}

// rawConfig mirrors the on-disk YAML. Pointers let us detect absent keys
// versus zero values (critical for bools: `start_tmux: false` must override
// the default `true`).
type rawConfig struct {
	TasksDir    *string  `yaml:"tasks_dir"`
	RepoRoots   []string `yaml:"repo_roots"`
	TmuxPrefix  *string  `yaml:"tmux_prefix"`
	DefaultBase *string  `yaml:"default_base"`
	DefaultType *string  `yaml:"default_type"`
	StartTmux   *bool    `yaml:"start_tmux"`
}

// DefaultPath resolves the config location. Honors $XDG_CONFIG_HOME, falls
// back to ~/.config/do-stuff/config.yaml.
func DefaultPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "do-stuff", "config.yaml")
}

// Load reads the YAML config at path. Missing file returns defaults with no
// error; parse failures return a *TaskError{Code: ConfigError}.
func Load(path string) (*Config, error) {
	cfg := defaults()

	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, &errs.TaskError{
			Code:    errs.ConfigError,
			Message: fmt.Sprintf("reading %s: %v", path, err),
			Details: map[string]any{"path": path},
		}
	}

	var raw rawConfig
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, &errs.TaskError{
			Code:    errs.ConfigError,
			Message: fmt.Sprintf("parsing %s: %v", path, err),
			Details: map[string]any{"path": path},
		}
	}

	apply(cfg, &raw)
	expandPaths(cfg)
	return cfg, nil
}

func defaults() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		TasksDir:    filepath.Join(home, ".do-stuff"),
		RepoRoots:   []string{},
		TmuxPrefix:  "task-",
		DefaultBase: "main",
		DefaultType: "feat",
		StartTmux:   true,
	}
}

// apply copies any explicitly-set fields from raw onto dst. Pointers are
// nil when the key is absent from YAML, so an explicit `start_tmux: false`
// overrides the default `true` while omitting the key leaves the default.
func apply(dst *Config, raw *rawConfig) {
	if raw.TasksDir != nil {
		dst.TasksDir = *raw.TasksDir
	}
	if raw.RepoRoots != nil {
		dst.RepoRoots = raw.RepoRoots
	}
	if raw.TmuxPrefix != nil {
		dst.TmuxPrefix = *raw.TmuxPrefix
	}
	if raw.DefaultBase != nil {
		dst.DefaultBase = *raw.DefaultBase
	}
	if raw.DefaultType != nil {
		dst.DefaultType = *raw.DefaultType
	}
	if raw.StartTmux != nil {
		dst.StartTmux = *raw.StartTmux
	}
}

func expandPaths(c *Config) {
	c.TasksDir = expandPath(c.TasksDir)
	for i, r := range c.RepoRoots {
		c.RepoRoots[i] = expandPath(r)
	}
}

func expandPath(p string) string {
	if p == "" {
		return p
	}
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	if p == "~" {
		return home
	}
	return os.ExpandEnv(p)
}
