package tmux_test

import (
	"os/exec"
	"testing"

	"github.com/jordangarrison/do-stuff/internal/tmux"
)

// requireTmux skips the test if tmux is not on PATH. Also sets
// TMUX_TMPDIR to a fresh dir so the session server lives in isolation
// and auto-tears-down with t.TempDir().
func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	t.Setenv("TMUX_TMPDIR", t.TempDir())
}

func TestAvailable_returnsNilWhenPresent(t *testing.T) {
	requireTmux(t)
	if err := tmux.Available(); err != nil {
		t.Fatalf("Available: %v", err)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	requireTmux(t)
	name := "ds-test-roundtrip"
	cwd := t.TempDir()
	t.Cleanup(func() { _ = tmux.KillSession(name) })

	has, err := tmux.HasSession(name)
	if err != nil || has {
		t.Fatalf("pre: HasSession=%v err=%v", has, err)
	}
	if err := tmux.NewSession(name, "first", cwd); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	has, err = tmux.HasSession(name)
	if err != nil || !has {
		t.Fatalf("post-new: HasSession=%v err=%v", has, err)
	}
	if err := tmux.NewWindow(name, "second", cwd); err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	// Detached session started with new-session -d should report attached=0.
	attached, err := tmux.IsSessionAttached(name)
	if err != nil {
		t.Fatalf("IsSessionAttached: %v", err)
	}
	if attached {
		t.Fatalf("expected detached, got attached")
	}
	if err := tmux.KillSession(name); err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	has, err = tmux.HasSession(name)
	if err != nil || has {
		t.Fatalf("post-kill: HasSession=%v err=%v", has, err)
	}
}
