package git

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

// BranchExistsLocal reports whether refs/heads/<branch> exists in repoPath.
func BranchExistsLocal(repoPath, branch string) (bool, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, &errs.TaskError{
			Code:    errs.GitError,
			Message: fmt.Sprintf("rev-parse %s failed: %v", branch, err),
			Details: map[string]any{"repo": repoPath, "branch": branch},
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
