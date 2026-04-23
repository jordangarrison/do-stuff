package task

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/tmux"
)

// AttachParams drives the Attach orchestrator. Mirrors CreateParams in shape.
type AttachParams struct {
	Slug       string
	TasksDir   string
	TmuxPrefix string
	StartTmux  bool             // --start-tmux: fabricate a session when metadata has none
	Now        func() time.Time // optional; defaults to time.Now
}

// AttachResult reports what Attach found or did. WasRecreated is true iff the
// orchestrator spawned a new tmux session to service this call.
type AttachResult struct {
	Task         *Task
	SessionName  string
	WasRecreated bool
}

// Attach loads task metadata, resolves the target tmux session, and
// recreates it from the saved repo list if the session has died. The
// caller is responsible for exec'ing `tmux attach` after this returns.
func Attach(p AttachParams) (*AttachResult, error) {
	if p.Now == nil {
		p.Now = time.Now
	}

	taskDir := filepath.Join(p.TasksDir, p.Slug)
	t, err := Load(taskDir)
	if err != nil {
		return nil, err
	}

	sessionName, err := resolveSessionName(t, p)
	if err != nil {
		return nil, err
	}

	if err := tmux.Available(); err != nil {
		return nil, err
	}

	has, err := tmux.HasSession(sessionName)
	if err != nil {
		return nil, err
	}
	if has {
		return &AttachResult{Task: t, SessionName: sessionName, WasRecreated: false}, nil
	}

	// Preflight: every recorded worktree dir must exist before we
	// ask tmux to cd into it. Upgrades raw tmux stderr into a
	// structured WorktreeMissing the caller can pattern-match.
	for _, r := range t.Repos {
		wp := filepath.Join(taskDir, r.Worktree)
		if _, err := os.Stat(wp); err != nil {
			if os.IsNotExist(err) {
				return nil, &errs.TaskError{
					Code:    errs.WorktreeMissing,
					Message: fmt.Sprintf("worktree directory missing for repo %q: %s", r.Name, wp),
					Details: map[string]any{"repo": r.Name, "path": wp, "slug": p.Slug},
				}
			}
			return nil, &errs.TaskError{
				Code:    errs.Internal,
				Message: fmt.Sprintf("stat worktree %s: %v", wp, err),
			}
		}
	}

	firstPath := filepath.Join(taskDir, t.Repos[0].Worktree)
	if err := tmux.NewSession(sessionName, t.Repos[0].Name, firstPath); err != nil {
		return nil, err
	}
	for i := 1; i < len(t.Repos); i++ {
		cwd := filepath.Join(taskDir, t.Repos[i].Worktree)
		if err := tmux.NewWindow(sessionName, t.Repos[i].Name, cwd); err != nil {
			return nil, err
		}
	}

	// Persist fabricated session names so future invocations don't need
	// --start-tmux again.
	if t.TmuxSession == "" {
		t.TmuxSession = sessionName
		if err := Write(taskDir, t); err != nil {
			return nil, err
		}
	}

	return &AttachResult{Task: t, SessionName: sessionName, WasRecreated: true}, nil
}

// resolveSessionName returns the session to use, or the canonical error
// when metadata lacks one and the caller did not opt into creation.
func resolveSessionName(t *Task, p AttachParams) (string, error) {
	if t.TmuxSession != "" {
		return t.TmuxSession, nil
	}
	if p.StartTmux {
		return p.TmuxPrefix + p.Slug, nil
	}
	return "", &errs.TaskError{
		Code:    errs.TmuxSessionMissing,
		Message: fmt.Sprintf("task %q has no tmux session; pass --start-tmux to create one", p.Slug),
		Details: map[string]any{
			"slug": p.Slug,
			"hint": "ds attach --start-tmux " + p.Slug,
		},
	}
}
