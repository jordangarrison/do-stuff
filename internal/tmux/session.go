// Package tmux wraps the `tmux` CLI with helpers sized for do-stuff's
// session-and-window needs. Public API takes no socket argument; tests
// isolate by setting TMUX_TMPDIR to a scratch dir.
package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

// Available returns a TaskError{TmuxUnavailable} if `tmux` is not on PATH.
func Available() error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return &errs.TaskError{
			Code:    errs.TmuxUnavailable,
			Message: "tmux not found on PATH",
		}
	}
	return nil
}

// HasSession reports whether a session with name exists.
func HasSession(name string) (bool, error) {
	cmd := exec.Command("tmux", "has-session", "-t", "="+name)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, wrapTmuxErr("has-session", err, nil)
	}
	return true, nil
}

// IsSessionAttached returns true when the named session has at least one
// client attached. Caller should have already verified existence.
//
// Unlike HasSession and KillSession, this uses a bare name (no "=" prefix)
// for -t. tmux 3.6a returns empty output for display-message -t =<name>
// when formatting #{session_attached}; the bare form works reliably.
func IsSessionAttached(name string) (bool, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("tmux", "display-message", "-p", "-t", name, "#{session_attached}")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return false, wrapTmuxErr("display-message", err, stderr.Bytes())
	}
	return strings.TrimSpace(stdout.String()) != "0", nil
}

// NewSession starts a detached session (-d) with one window named
// firstWindowName, cwd defaulted to the provided path. Errors if a
// session with the same name already exists.
func NewSession(name, firstWindowName, cwd string) error {
	return run("new-session", "-d", "-s", name, "-n", firstWindowName, "-c", cwd)
}

// NewWindow appends a window named windowName to an existing session.
func NewWindow(session, windowName, cwd string) error {
	return run("new-window", "-t", "="+session, "-n", windowName, "-c", cwd)
}

// KillSession tears down the named session. No-op error suppression is
// the caller's job; tmux returns exit 1 for missing sessions which we
// surface as a TmuxError so callers can distinguish.
func KillSession(name string) error {
	return run("kill-session", "-t", "="+name)
}

func run(args ...string) error {
	cmd := exec.Command("tmux", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return wrapTmuxErr(args[0], err, stderr.Bytes())
	}
	return nil
}

func wrapTmuxErr(op string, err error, stderr []byte) error {
	return &errs.TaskError{
		Code:    errs.TmuxUnavailable, // caller may narrow; default is generic tmux failure
		Message: fmt.Sprintf("tmux %s failed: %v", op, err),
		Details: map[string]any{
			"op":          op,
			"tmux_stderr": strings.TrimSpace(string(stderr)),
		},
	}
}
