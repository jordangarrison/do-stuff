// Package git wraps the `git` CLI with focused helpers for worktree and
// branch operations. All functions shell out with explicit -C <repoPath>
// so callers never chdir. Errors are returned as *errs.TaskError{GitError}
// carrying the command's stderr in Details["git_stderr"] for diagnostics.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"

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
	// tracking branch in the new worktree. Remote is hardcoded to "origin";
	// v0.1 does not support other remotes.
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
	err := runGit(repoPath, args...)
	if err == nil {
		return nil
	}
	// Narrow path-collision failures to WorktreeExists so the CLI layer
	// can surface the spec's exit 5 without pattern-matching stderr.
	var te *errs.TaskError
	if errors.As(err, &te) {
		if stderr, _ := te.Details["git_stderr"].(string); strings.Contains(stderr, "already exists") {
			te.Code = errs.WorktreeExists
		}
	}
	return err
}

// WorktreeDirty reports whether worktreePath has uncommitted changes.
// Implementation: `git -C worktreePath status --porcelain`; non-empty stdout = dirty.
func WorktreeDirty(worktreePath string) (bool, error) {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return false, &errs.TaskError{
			Code:    errs.GitError,
			Message: fmt.Sprintf("git status failed in %s: %v", worktreePath, err),
			Details: map[string]any{
				"worktree":   worktreePath,
				"git_stderr": stderr.String(),
			},
		}
	}
	return stdout.Len() > 0, nil
}

// WorktreeRemove shells out `git -C repoPath worktree remove [--force] worktreePath`.
// force=true maps to --force (required when the worktree has local changes or is locked).
func WorktreeRemove(repoPath, worktreePath string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreePath)
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
