package task

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jordangarrison/do-stuff/internal/errs"
	dsgit "github.com/jordangarrison/do-stuff/internal/git"
	"github.com/jordangarrison/do-stuff/internal/tmux"
)

// CreateParams describes everything Create needs to realize a task.
// Nothing in this struct is derived at call time — resolution (e.g. of
// repo names into absolute paths) happens in the cli layer.
type CreateParams struct {
	Slug           string
	Type           string
	Ticket         string
	TicketURL      string
	BranchOverride string
	Base           string
	TasksDir       string
	Repos          []ResolvedRepo
	NoTmux         bool
	StartTmux      bool
	Strict         bool
	TmuxPrefix     string
	Now            func() time.Time // optional; defaults to time.Now
}

// ResolvedRepo is a repo name matched against discovery, with its absolute
// source path attached. Create operates exclusively on these.
type ResolvedRepo struct {
	Name string
	Path string
}

// CreateResult is what Create returns on success. RepoStates is
// index-aligned with CreateParams.Repos.
type CreateResult struct {
	Task       *Task
	RepoStates []RepoState
	TaskDir    string
}

// RepoState captures which AddMode was applied to a particular repo so
// the cli layer can render branch_state in the success envelope.
type RepoState struct {
	Name         string
	WorktreePath string
	BranchState  string // "created" | "checked_out_existing" | "fetched_tracking"
}

// Create runs the full ds new flow: derive branch, ensure task dir is
// free, add a worktree per repo, optionally start a tmux session with
// one window per repo, and write .task.json.
//
// On failure after some worktrees have been created, those worktrees
// stay on disk and .task.json is not written. ds finish (v0.2) or
// manual cleanup is expected.
func Create(p CreateParams) (*CreateResult, error) {
	if p.Now == nil {
		p.Now = time.Now
	}
	if len(p.Repos) == 0 {
		return nil, &errs.TaskError{Code: errs.InvalidArgs, Message: "no repos provided"}
	}

	branch := p.BranchOverride
	if branch == "" {
		branch = deriveBranch(p.Type, p.Ticket, p.Slug)
	}

	taskDir := filepath.Join(p.TasksDir, p.Slug)
	if _, err := os.Stat(taskDir); err == nil {
		return nil, &errs.TaskError{
			Code:    errs.TaskExists,
			Message: fmt.Sprintf("task directory already exists: %s", taskDir),
			Details: map[string]any{"path": taskDir, "slug": p.Slug},
		}
	} else if !os.IsNotExist(err) {
		return nil, &errs.TaskError{
			Code:    errs.Internal,
			Message: fmt.Sprintf("stat %s: %v", taskDir, err),
		}
	}

	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return nil, &errs.TaskError{
			Code:    errs.Internal,
			Message: fmt.Sprintf("mkdir %s: %v", taskDir, err),
		}
	}

	states := make([]RepoState, 0, len(p.Repos))
	repoRefs := make([]RepoRef, 0, len(p.Repos))

	for _, repo := range p.Repos {
		mode, state, err := pickAddMode(repo.Path, branch, p.Strict)
		if err != nil {
			return nil, err
		}
		worktreePath := filepath.Join(taskDir, repo.Name)
		if err := dsgit.WorktreeAdd(repo.Path, worktreePath, branch, p.Base, mode); err != nil {
			return nil, err
		}
		states = append(states, RepoState{Name: repo.Name, WorktreePath: worktreePath, BranchState: state})
		repoRefs = append(repoRefs, RepoRef{Name: repo.Name, Path: repo.Path, Worktree: repo.Name})
	}

	sessionName := ""
	if p.StartTmux && !p.NoTmux {
		sessionName = p.TmuxPrefix + p.Slug
		if err := tmux.Available(); err != nil {
			return nil, err
		}
		has, err := tmux.HasSession(sessionName)
		if err != nil {
			return nil, err
		}
		if has {
			return nil, &errs.TaskError{
				Code:    errs.TmuxSessionExists,
				Message: fmt.Sprintf("tmux session %q already exists", sessionName),
				Details: map[string]any{"session": sessionName},
			}
		}
		if err := tmux.NewSession(sessionName, p.Repos[0].Name, states[0].WorktreePath); err != nil {
			return nil, err
		}
		for i := 1; i < len(p.Repos); i++ {
			if err := tmux.NewWindow(sessionName, p.Repos[i].Name, states[i].WorktreePath); err != nil {
				return nil, err
			}
		}
	}

	t := &Task{
		Slug:        p.Slug,
		Type:        p.Type,
		Ticket:      p.Ticket,
		TicketURL:   p.TicketURL,
		Branch:      branch,
		Base:        p.Base,
		Repos:       repoRefs,
		TmuxSession: sessionName,
		CreatedAt:   p.Now().UTC(),
	}
	if err := Write(taskDir, t); err != nil {
		return nil, err
	}

	return &CreateResult{Task: t, RepoStates: states, TaskDir: taskDir}, nil
}

// deriveBranch renders `{type}/[{ticket-lower}-]{slug}` per SPEC.
func deriveBranch(typ, ticket, slug string) string {
	if ticket == "" {
		return typ + "/" + slug
	}
	return typ + "/" + strings.ToLower(ticket) + "-" + slug
}

// pickAddMode inspects the repo's local + remote branch state and
// returns the matching AddMode plus a human-facing state label. Strict
// callers rejecting any reuse get a BranchConflict instead.
func pickAddMode(repoPath, branch string, strict bool) (dsgit.AddMode, string, error) {
	hasLocal, err := dsgit.BranchExistsLocal(repoPath, branch)
	if err != nil {
		return 0, "", err
	}
	if hasLocal {
		if strict {
			return 0, "", &errs.TaskError{
				Code:    errs.BranchConflict,
				Message: fmt.Sprintf("branch %q already exists locally in %s (--strict)", branch, repoPath),
				Details: map[string]any{"repo": repoPath, "branch": branch, "scope": "local"},
			}
		}
		return dsgit.CheckoutExisting, "checked_out_existing", nil
	}
	// Skip the remote check entirely when the repo has no origin configured;
	// BranchExistsRemote would fail with exit 128 ("origin does not appear to
	// be a git repository") instead of a clean "not found" signal.
	if !hasOrigin(repoPath) {
		return dsgit.CreateFromBase, "created", nil
	}
	hasRemote, err := dsgit.BranchExistsRemote(repoPath, "origin", branch)
	if err != nil {
		return 0, "", err
	}
	if hasRemote {
		if strict {
			return 0, "", &errs.TaskError{
				Code:    errs.BranchConflict,
				Message: fmt.Sprintf("branch %q already exists on origin for %s (--strict)", branch, repoPath),
				Details: map[string]any{"repo": repoPath, "branch": branch, "scope": "remote"},
			}
		}
		return dsgit.FetchAndTrack, "fetched_tracking", nil
	}
	return dsgit.CreateFromBase, "created", nil
}

// hasOrigin returns true when repoPath has an "origin" remote configured.
// Uses `git config --get remote.origin.url`: exit 0 = present, non-zero = absent.
func hasOrigin(repoPath string) bool {
	cmd := exec.Command("git", "-C", repoPath, "config", "--get", "remote.origin.url")
	return cmd.Run() == nil
}
