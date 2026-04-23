package errs

// Code is a stable, enumerated error identifier emitted in the JSON envelope.
type Code string

const (
	InvalidArgs        Code = "invalid_args"
	RepoNotFound       Code = "repo_not_found"
	TaskExists         Code = "task_exists"
	TaskNotFound       Code = "task_not_found"
	BranchConflict     Code = "branch_conflict"
	WorktreeExists     Code = "worktree_exists"
	WorktreeDirty      Code = "worktree_dirty"
	TmuxUnavailable    Code = "tmux_unavailable"
	TmuxSessionExists  Code = "tmux_session_exists"
	TmuxSessionMissing Code = "tmux_session_not_found"
	GitError           Code = "git_error"
	ConfigError        Code = "config_error"
	Internal           Code = "internal_error"
)

// TaskError is the structured failure returned by core packages and surfaced
// verbatim in the CLI's JSON envelope. Details is per-code free-form and
// documented in the corresponding command / package.
type TaskError struct {
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *TaskError) Error() string { return e.Message }

// ExitCode maps a Code to the process exit code documented in the spec.
// Unknown codes fall through to 1 so a wrongly-constructed TaskError never
// pretends to be success.
func (e *TaskError) ExitCode() int {
	switch e.Code {
	case InvalidArgs:
		return 2
	case RepoNotFound:
		return 3
	case TaskExists:
		return 4
	case BranchConflict, WorktreeExists:
		return 5
	case TmuxUnavailable, TmuxSessionExists, TmuxSessionMissing:
		return 6
	case GitError, WorktreeDirty:
		return 7
	case ConfigError:
		return 8
	case TaskNotFound:
		return 9
	case Internal:
		return 1
	default:
		return 1
	}
}
