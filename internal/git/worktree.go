// Package git wraps the `git` CLI with focused helpers for worktree and
// branch operations. All functions shell out with explicit -C <repoPath>
// so callers never chdir. Errors are returned as *errs.TaskError{GitError}
// carrying the command's stderr in Details["git_stderr"] for diagnostics.
package git

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

// AddMode selects the shape of `git worktree add` a caller wants.
type AddMode int

const (
	// CreateFromBase creates a new branch off base and adds a worktree on it.
	CreateFromBase AddMode = iota
	// CheckoutExisting checks out an already-existing local branch into the worktree.
	CheckoutExisting
	// FetchAndTrack fetches the branch from origin and creates a local
	// tracking branch in the new worktree.
	FetchAndTrack
)

// WorktreeAdd runs the appropriate `git worktree add` form for mode.
// branch is the target branch name (without refs/heads/ prefix). base is
// only read when mode == CreateFromBase.
func WorktreeAdd(repoPath, worktreePath, branch, base string, mode AddMode) error {
	var args []string
	switch mode {
	case CreateFromBase:
		args = []string{"worktree", "add", "-b", branch, worktreePath, base}
	case CheckoutExisting:
		args = []string{"worktree", "add", worktreePath, branch}
	case FetchAndTrack:
		if err := FetchBranch(repoPath, "origin", branch); err != nil {
			return err
		}
		args = []string{"worktree", "add", "--track", "-b", branch, worktreePath, "origin/" + branch}
	default:
		return &errs.TaskError{
			Code:    errs.GitError,
			Message: fmt.Sprintf("unknown AddMode %d", mode),
		}
	}
	return runGit(repoPath, args...)
}

// runGit invokes `git -C repo <args...>`, returning a GitError on failure
// with stderr attached to Details.
func runGit(repoPath string, args ...string) error {
	full := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &errs.TaskError{
			Code:    errs.GitError,
			Message: fmt.Sprintf("git %v failed: %v", args, err),
			Details: map[string]any{
				"repo":       repoPath,
				"cmd":        args,
				"git_stderr": stderr.String(),
			},
		}
	}
	return nil
}
