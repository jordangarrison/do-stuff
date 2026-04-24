package task

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jordangarrison/do-stuff/internal/errs"
	dsgit "github.com/jordangarrison/do-stuff/internal/git"
	"github.com/jordangarrison/do-stuff/internal/tmux"
)

// FinishParams drives the Finish orchestrator. Shape mirrors CreateParams.
type FinishParams struct {
	Slug         string
	TasksDir     string
	Force        bool
	KeepBranches bool
	Now          func() time.Time // optional; defaults to time.Now (reserved)
}

// FinishResult reports what Finish tore down. RemovedWorktrees is index
// order of Task.Repos and includes already-missing dirs (idempotent shape).
type FinishResult struct {
	Task             *Task
	RemovedWorktrees []string
	KilledSession    string
	BranchesKept     bool
}

// Finish tears down a task: verifies clean worktrees (unless Force), removes
// each worktree from its source repo, kills the tmux session if present,
// optionally deletes the task branch in every source repo, and removes the
// task directory. Partial failures halt; the user re-runs.
func Finish(p FinishParams) (*FinishResult, error) {
	if p.Now == nil {
		p.Now = time.Now
	}

	taskDir := filepath.Join(p.TasksDir, p.Slug)
	t, err := Load(taskDir)
	if err != nil {
		return nil, err
	}

	// Dirty preflight (unless forced). Skips worktrees whose dirs are
	// already gone — missing is treated as already-clean for the user's
	// intent: "this task goes away."
	if !p.Force {
		for _, repo := range t.Repos {
			wt := filepath.Join(taskDir, repo.Worktree)
			if _, err := os.Stat(wt); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, &errs.TaskError{
					Code:    errs.Internal,
					Message: fmt.Sprintf("stat worktree %s: %v", wt, err),
				}
			}
			dirty, err := dsgit.WorktreeDirty(wt)
			if err != nil {
				return nil, err
			}
			if dirty {
				return nil, &errs.TaskError{
					Code:    errs.WorktreeDirty,
					Message: fmt.Sprintf("worktree %q is dirty; pass --force to discard", repo.Name),
					Details: map[string]any{"repo": repo.Name, "path": wt},
				}
			}
		}
	}

	// State mutation begins here.
	removed := make([]string, 0, len(t.Repos))
	for _, repo := range t.Repos {
		wt := filepath.Join(taskDir, repo.Worktree)
		_, statErr := os.Stat(wt)
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, &errs.TaskError{
				Code:    errs.Internal,
				Message: fmt.Sprintf("stat worktree %s: %v", wt, statErr),
			}
		}
		// When the worktree dir is already gone, call WorktreeRemove with
		// force=true to prune the git worktree registration so that
		// BranchDelete can succeed (git refuses to delete a branch that is
		// still checked out in a registered-but-missing worktree).
		force := p.Force || os.IsNotExist(statErr)
		if err := dsgit.WorktreeRemove(repo.Path, wt, force); err != nil {
			return nil, err
		}
		removed = append(removed, repo.Name)
	}

	killedSession := ""
	if t.TmuxSession != "" {
		if err := tmux.Available(); err == nil {
			has, err := tmux.HasSession(t.TmuxSession)
			if err != nil {
				return nil, err
			}
			if has {
				if err := tmux.KillSession(t.TmuxSession); err != nil {
					return nil, err
				}
				killedSession = t.TmuxSession
			}
		}
	}

	if !p.KeepBranches {
		for _, repo := range t.Repos {
			if err := dsgit.BranchDelete(repo.Path, t.Branch, p.Force); err != nil {
				return nil, err
			}
		}
	}

	if err := os.RemoveAll(taskDir); err != nil {
		return nil, &errs.TaskError{
			Code:    errs.Internal,
			Message: fmt.Sprintf("remove task dir %s: %v", taskDir, err),
		}
	}

	return &FinishResult{
		Task:             t,
		RemovedWorktrees: removed,
		KilledSession:    killedSession,
		BranchesKept:     p.KeepBranches,
	}, nil
}
