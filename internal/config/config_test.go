package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

func TestLoad_missingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load("/nonexistent/path/does/not/exist.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TasksDir == "" {
		t.Fatal("TasksDir should have a default")
	}
	if cfg.TmuxPrefix != "task-" {
		t.Fatalf("want task-, got %q", cfg.TmuxPrefix)
	}
	if cfg.DefaultBase != "main" {
		t.Fatalf("want main, got %q", cfg.DefaultBase)
	}
	if cfg.DefaultType != "feat" {
		t.Fatalf("want feat, got %q", cfg.DefaultType)
	}
	if !cfg.StartTmux {
		t.Fatal("StartTmux should default true")
	}
	if len(cfg.RepoRoots) != 0 {
		t.Fatalf("RepoRoots should default empty, got %v", cfg.RepoRoots)
	}
}

func TestLoad_validFileParses(t *testing.T) {
	cfg, err := Load("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.RepoRoots) != 2 {
		t.Fatalf("want 2 roots, got %d", len(cfg.RepoRoots))
	}
	home, _ := os.UserHomeDir()
	if cfg.TasksDir != filepath.Join(home, ".do-stuff") {
		t.Fatalf("want tilde expanded, got %q", cfg.TasksDir)
	}
	if cfg.RepoRoots[1] != filepath.Join(home, "dev/personal") {
		t.Fatalf("want $HOME expanded, got %q", cfg.RepoRoots[1])
	}
}

func TestLoad_invalidYAMLReturnsConfigError(t *testing.T) {
	_, err := Load("testdata/invalid.yaml")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) {
		t.Fatalf("want *errs.TaskError, got %T", err)
	}
	if te.Code != errs.ConfigError {
		t.Fatalf("want ConfigError, got %s", te.Code)
	}
}

func TestExpandPath_tilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandPath("~/foo")
	if got != filepath.Join(home, "foo") {
		t.Fatalf("want %s/foo, got %s", home, got)
	}
}

func TestExpandPath_home(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandPath("$HOME/foo")
	if got != filepath.Join(home, "foo") {
		t.Fatalf("want %s/foo, got %s", home, got)
	}
}

func TestExpandPath_absoluteUnchanged(t *testing.T) {
	got := expandPath("/abs/path")
	if got != "/abs/path" {
		t.Fatalf("want /abs/path, got %s", got)
	}
}

func TestDefaultPath_honorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	p := DefaultPath()
	if p != "/tmp/xdg-test/do-stuff/config.yaml" {
		t.Fatalf("want /tmp/xdg-test/do-stuff/config.yaml, got %s", p)
	}
}

func TestDefaultPath_fallsBackToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	p := DefaultPath()
	want := filepath.Join(home, ".config/do-stuff/config.yaml")
	if p != want {
		t.Fatalf("want %s, got %s", want, p)
	}
}
