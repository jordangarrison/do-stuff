package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

// BranchExistsLocal reports whether refs/heads/<branch> exists in repoPath.
func BranchExistsLocal(repoPath, branch string) (bool, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, &errs.TaskError{
			Code:    errs.GitError,
			Message: fmt.Sprintf("rev-parse %s failed: %v", branch, err),
			Details: map[string]any{
				"repo":       repoPath,
				"branch":     branch,
				"git_stderr": stderr.String(),
			},
		}
	}
	return true, nil
}

// BranchExistsRemote reports whether <remote> advertises refs/heads/<branch>.
// Uses `git ls-remote --exit-code`: exit 0 = present, exit 2 = absent.
func BranchExistsRemote(repoPath, remote, branch string) (bool, error) {
	cmd := exec.Command("git", "-C", repoPath, "ls-remote", "--exit-code", "--heads", remote, branch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
		return false, nil
	}
	return false, &errs.TaskError{
		Code:    errs.GitError,
		Message: fmt.Sprintf("ls-remote %s %s failed: %v", remote, branch, err),
		Details: map[string]any{
			"repo":       repoPath,
			"remote":     remote,
			"branch":     branch,
			"git_stderr": stderr.String(),
		},
	}
}

// FetchBranch runs `git fetch <remote> <branch>` scoped to a single ref.
func FetchBranch(repoPath, remote, branch string) error {
	return runGit(repoPath, "fetch", "--quiet", "--no-tags", remote, branch)
}

// BranchDelete shells out `git -C repoPath branch -d|-D branch`.
// force=true maps to -D (delete unmerged branches). Missing-branch stderr
// is swallowed (returns nil) so callers can re-run idempotently.
func BranchDelete(repoPath, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	cmd := exec.Command("git", "-C", repoPath, "branch", flag, branch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		s := stderr.String()
		if strings.Contains(s, "not found") {
			return nil
		}
		return &errs.TaskError{
			Code:    errs.GitError,
			Message: fmt.Sprintf("git branch %s %s failed: %v", flag, branch, err),
			Details: map[string]any{
				"repo":       repoPath,
				"branch":     branch,
				"git_stderr": s,
			},
		}
	}
	return nil
}

// HasOrigin reports whether repoPath has an "origin" remote configured.
// Useful to decide whether to call BranchExistsRemote; on a repo without
// origin, that helper fails with exit 128 rather than a clean "absent".
func HasOrigin(repoPath string) bool {
	cmd := exec.Command("git", "-C", repoPath, "config", "--get", "remote.origin.url")
	return cmd.Run() == nil
}
