# v0.2 Slice 1 — `ds finish` (+ PR #4 carry-overs + AGENTS.md)

**Status:** approved 2026-04-24
**Scope:** this doc covers v0.2 slice 1 only. v0.1 shipped via PR #4 (merge `bcfc33f`).
**Prior art:** `docs/superpowers/specs/2026-04-23-v0-1-slice-2b-design.md`, `docs/superpowers/plans/2026-04-23-v0-1-slice-2b.md`. Canonical spec: `SPEC.md` v0.5.

## Goal

Ship the last command on the v0.1 → v0.2 path:

- `ds finish <slug> [--force] [--keep-branches]` — remove worktrees, kill the tmux session, optionally delete branches, delete the task dir. Idempotent re-run after partial failure.

Slice 1 also lands three PR #4 carry-over refactors and an AGENTS.md so the working mentality is explicit before v0.2's larger pieces (`create-interactive`, derive-from-filesystem metadata, post-create hooks) arrive.

## Non-goals for this slice

- `ds create-interactive` — separate v0.2 slice.
- Derive-from-filesystem metadata model. Still tracked as a v0.2 candidate per slice-2b's open questions. Not in scope here.
- Remote branch deletion (`git push origin --delete <branch>`). `--keep-branches` toggles local delete only. A future `--delete-remote` flag can open this door if demand emerges.
- Transactional rollback on partial failure. Halt-on-error matches `task.Create`'s shape; re-run is the recovery path.
- Narrower tmux error codes at callsites. The parameterized `wrapTmuxErr` from slice 2b stays funnelled through `TmuxUnavailable` here; adding a generic `tmux_error` is additive and belongs in a later pass.

## PR #4 carry-overs

Land as the first four commits on the slice-1 branch so feature commits can cite them. AGENTS.md first because it codifies the rules the refactors exemplify.

### Carry-over 0: AGENTS.md (+ CLAUDE.md symlink)

Root-level `AGENTS.md` documents the mentality the project rewards. `CLAUDE.md` is a symlink to the same file so Claude Code picks it up automatically.

Sections:

- **Philosophy** — Hickey *Simple Made Easy* (simple = decomplected, not familiar), grugbrain.dev (complexity demon, three similar lines beats a bad abstraction, YAGNI), Unix compose (this tool is a thin Go wrapper over `git` + `tmux` + `fzf`; don't reinvent).
- **Repo shape** — three layers (core → cli → skills), each calls only the layer beneath. Orchestrators in `internal/task/*` compose primitives from `internal/git` + `internal/tmux`; no cross-primitive calls. Envelope is the contract: error codes stable, messages/details free-form. One file, one job. "State mutation begins here" marker comments when a function crosses from preflight → effects (mirror `task.Create`'s pattern).
- **Anti-patterns** — premature abstraction, hidden state, backwards-compat shims, error handling for impossible cases, narrating decisions in comments instead of PR descriptions.
- **Testing** — hermetic by default. Gate tmux/git-requiring tests with presence checks (`exec.LookPath`). Inject `execFn` / `lookupFn` / `selectorFn` for shell-outs; never rely on real `syscall.Exec` in tests.
- **Commits** — conventional commits with project-specific scopes (`tmux`, `git`, `task`, `cli`, `errs`, `docs`). Every commit keeps `go test ./...` green. No Claude co-author.
- **Agent workflow** — skills live in `skills/`. Design flow: `superpowers:brainstorming` → `superpowers:writing-plans` → `superpowers:subagent-driven-development` or `superpowers:executing-plans`. Specs land in `docs/superpowers/specs/`, plans in `docs/superpowers/plans/`.

AGENTS.md stays short and declarative — aspirational prose is anti-grug. Link out to references rather than reproducing them.

### Carry-over 1: shared slug regex

`slugRe` in `internal/cli/new.go` and `attachSlugRe` in `internal/cli/attach.go` are identical (`^[a-z0-9][a-z0-9._-]*$`). `ds finish` needs the same validation. Extract into `internal/cli/slug.go`:

```go
package cli

import "regexp"

// taskSlugRe is the validation regex for a task slug. Kept here so every
// command (new, attach, finish, …) validates identically.
var taskSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
```

`new.go` drops its local `slugRe`; `attach.go` drops `attachSlugRe`; both reference `taskSlugRe`. No behavior change.

### Carry-over 2: shared tmux session-state probe

`probeSessionState` lives in `internal/cli/pick.go`; `internal/cli/list.go::buildListTask` reimplements the same 12-line logic inline. Extract into `internal/cli/tmux_state.go`:

```go
package cli

import "github.com/jordangarrison/do-stuff/internal/tmux"

// probeSessionState reports the live state of a tmux session: "attached",
// "detached", or "absent". Absent covers both "tmux not on PATH" and
// "session does not exist" so callers don't have to fork.
func probeSessionState(session string) string {
    if err := tmux.Available(); err != nil {
        return "absent"
    }
    has, err := tmux.HasSession(session)
    if err != nil || !has {
        return "absent"
    }
    attached, err := tmux.IsSessionAttached(session)
    switch {
    case err != nil:
        return "detached"
    case attached:
        return "attached"
    default:
        return "detached"
    }
}
```

`pick.go` deletes its copy. `list.go::buildListTask` replaces its inline block with `state := probeSessionState(t.TmuxSession)` guarded on `t.TmuxSession != ""` (else `"absent"`). The list-side `tmuxAvailable` short-circuit disappears; the helper's own `tmux.Available()` check is cheap enough.

### Carry-over 3: shared tasks-dir scan

`internal/cli/pick.go::loadAllTasks` and the inline loop in `internal/cli/list.go::runList` both walk `<tasksDir>/*/` filtering for `.task.json`, loading each, warning on integrity errors. Extract into `internal/cli/tasks_scan.go`:

```go
package cli

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/jordangarrison/do-stuff/internal/errs"
    "github.com/jordangarrison/do-stuff/internal/task"
)

// scanTasks walks tasksDir, returning one *task.Task per subdirectory that
// contains a readable .task.json. warn (nullable) is called for entries
// that fail to load; the scan continues past them.
//
// A missing tasksDir returns (nil, nil). Other readdir failures return a
// ConfigError.
func scanTasks(tasksDir string, warn func(taskPath string, err error)) ([]*task.Task, error) {
    entries, err := os.ReadDir(tasksDir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil
        }
        return nil, &errs.TaskError{
            Code:    errs.ConfigError,
            Message: fmt.Sprintf("reading tasks_dir %s: %v", tasksDir, err),
            Details: map[string]any{"path": tasksDir},
        }
    }
    out := make([]*task.Task, 0, len(entries))
    for _, e := range entries {
        if !e.IsDir() {
            continue
        }
        taskPath := filepath.Join(tasksDir, e.Name())
        if _, err := os.Stat(filepath.Join(taskPath, task.MetadataFile)); err != nil {
            continue
        }
        t, err := task.Load(taskPath)
        if err != nil {
            if warn != nil {
                warn(taskPath, err)
            }
            continue
        }
        out = append(out, t)
    }
    return out, nil
}
```

`pick.go::loadAllTasks` becomes a one-liner wrapper (or is deleted and callers call `scanTasks` directly). `list.go::runList` replaces its inline block with a `scanTasks` call + a `warn` closure.

`finish` does not consume `scanTasks` — it loads a single task by slug via `task.Load` — but while the tasks-dir scan is getting touched anyway, dedup is free.

## New `internal/git` primitives

Added to `internal/git`. All three shell out via the existing `runGit` helper and return `GitError` on failure.

```go
// WorktreeRemove shells out `git -C repoPath worktree remove [--force] worktreePath`.
// force=true maps to --force (required when the worktree has local changes or is locked).
func WorktreeRemove(repoPath, worktreePath string, force bool) error

// WorktreeDirty reports whether the worktree has uncommitted changes.
// Implementation: `git -C worktreePath status --porcelain`; non-empty output = dirty.
func WorktreeDirty(worktreePath string) (bool, error)

// BranchDelete shells out `git -C repoPath branch -d|-D branch`.
// force=true maps to -D (delete unmerged branches). Missing-branch stderr is
// swallowed (returns nil) so finish re-runs stay idempotent.
func BranchDelete(repoPath, branch string, force bool) error
```

`BranchDelete`'s "not found" swallow matches the existing stderr-substring pattern that slice 2b introduced for `WorktreeAdd` handling "already exists". Substring checked: `"not found"` (git's message is stable: `error: branch '<name>' not found.`).

`WorktreeDirty` uses `runGit`-style execution but needs stdout, not just stderr. It gets its own small helper or an inline `exec.Command` wrapping — implementer's pick, smaller diff wins.

## `internal/task` — `Finish` orchestrator

Mirrors the shape of `Create` and `Attach`. Owns the sequencing of metadata load → dirty preflight → worktree removal → session kill → branch delete → task-dir removal. Does not know about envelopes or cobra.

```go
type FinishParams struct {
    Slug         string
    TasksDir     string
    Force        bool
    KeepBranches bool
    Now          func() time.Time // optional; defaults to time.Now (reserved; no current use)
}

type FinishResult struct {
    Task             *Task
    RemovedWorktrees []string // repo.Name order; includes already-missing dirs
    KilledSession    string   // "" if nothing was killed
    BranchesKept     bool
}

func Finish(p FinishParams) (*FinishResult, error)
```

### Flow

1. `taskDir := filepath.Join(p.TasksDir, p.Slug)`; `t, err := Load(taskDir)`. Propagate `TaskNotFound` on miss.
2. **Dirty preflight** (skipped when `p.Force`): for each `repo` in `t.Repos`, compute `wtPath := filepath.Join(taskDir, repo.Worktree)`. If the dir exists, call `git.WorktreeDirty(wtPath)`. First dirty → return `WorktreeDirty` error with `details: {repo, path}`, exit 7. No mutation yet. Missing worktree dirs are skipped (no stat error to raise; idempotent re-run tolerates them).
3. **Remove worktrees**: for each `repo`, stat `wtPath`.
   - Present → `git.WorktreeRemove(repo.Path, wtPath, p.Force)`. On success, append `repo.Name` to `RemovedWorktrees`. On error, return immediately — `RemovedWorktrees` captures what succeeded before the failure. Remaining repos are left untouched (halt-on-error).
   - Missing → skip the shell-out, append `repo.Name` to `RemovedWorktrees` anyway (user intent = "this repo is removed from the task"; target state matches).
4. **Kill session** when `t.TmuxSession != ""`:
   - `tmux.Available()` fails → skip silently; `KilledSession` stays `""`.
   - `tmux.HasSession(name)` returns `(false, nil)` → skip; `KilledSession` stays `""`.
   - `HasSession` errors → surface as `TmuxUnavailable`; halt.
   - Session present → `tmux.KillSession(name)`; on success set `KilledSession = name`.
5. **Delete branches** unless `p.KeepBranches`: for each `repo`, `git.BranchDelete(repo.Path, t.Branch, p.Force)`. Missing-branch is already swallowed inside the primitive. `-d` refusing an unmerged branch → `GitError` (exit 7); user opts back in via `--force`.
6. **Remove task dir**: `os.RemoveAll(taskDir)`. At this point only `.task.json` and an empty dir should remain; `RemoveAll` handles both.
7. Return `&FinishResult{Task: t, RemovedWorktrees: …, KilledSession: …, BranchesKept: p.KeepBranches}`.

`Finish` is pure orchestration; it does no exec, no envelope, no cobra wiring.

### Invariant: preflight before mutation

Dirty preflight runs to completion before any `WorktreeRemove`. A failure there leaves nothing on disk changed — matches slice-2b's `preflightWorktrees` contract on `Attach`.

## Command: `ds finish`

### Signature

```
ds finish <slug> [--force] [--keep-branches]
```

### Envelope `data` (success)

```json
{
  "slug": "infra-6700-auth-refactor",
  "removed_worktrees": ["api", "web"],
  "killed_session": "task-infra-6700-auth-refactor",
  "branches_kept": false
}
```

- `killed_session` uses `omitempty` — dropped from the JSON when nothing was killed. Matches the `tmux_session` field's treatment in `NewData` / `AttachData`.
- `branches_kept` is literally the user's flag choice; signals *what was asked for*, not *what was deleted*. When a repo's branch was already gone and got swallowed, the field still reports `false` because the user asked to delete.

### TTY vs piped

No process replacement. `finish` always emits an envelope on success. TTY mode renders the human summary; piped mode emits JSON.

### Error paths

| condition | code | exit |
|---|---|---|
| slug fails validation | `invalid_args` | 2 |
| `<tasks_dir>/<slug>/.task.json` missing or unreadable | `task_not_found` | 9 |
| any worktree dirty, no `--force` | `worktree_dirty` | 7 |
| `git worktree remove` fails | `git_error` | 7 |
| `git branch -d` refuses unmerged, no `--force` | `git_error` | 7 |
| tmux kill fails mid-flight | `tmux_unavailable` | 6 |
| config load fails | `config_error` | 8 |

### `--force` semantics (locked)

Single escape hatch. `--force` toggles **all three** of:

- Skip the dirty preflight.
- Pass `--force` to `git worktree remove`.
- Use `git branch -D` (delete unmerged).

Rationale: `--force` as a single "I know, just do it" is the common shape; the happy path stays strict (dirty → exit 7, unmerged → exit 7), so accidents cost nothing.

### `--keep-branches` semantics (locked)

Skips step 5 entirely. Branches stay where they are in each source repo. `branches_kept = true`.

Remote-branch deletion is explicitly out of scope. A future `--delete-remote` flag is the shape if that capability is ever needed.

## Output layering

Same shape as slices 2a / 2b:

- `NewFinishCmd(flags *GlobalFlags)`; package-private `runFinish(opts)`.
- `RunE` wraps non-zero `runFinish` into `&ExitError{code}`.
- `cmd.OutOrStdout()` / `cmd.ErrOrStderr()` threaded into opts.
- `finishOpts` carries a `FinishFn` field (defaults to `task.Finish`) so tests don't need real git/tmux.

## Testing strategy

### `internal/git/worktree_test.go`

- `TestWorktreeRemove_clean` — add a worktree, remove it, assert dir gone.
- `TestWorktreeRemove_dirtyForced` — add a worktree, modify a file, call with `force=true`, assert dir gone.
- `TestWorktreeRemove_dirtyUnforced` — same but `force=false`, assert `GitError`.
- `TestWorktreeDirty_cleanFalse` / `TestWorktreeDirty_dirtyTrue`.

### `internal/git/branch_test.go`

- `TestBranchDelete_merged` — create branch off base, delete with `force=false`, assert gone.
- `TestBranchDelete_unmergedForced` — branch with unmerged commits, `force=true`, assert gone.
- `TestBranchDelete_unmergedUnforced` — `force=false`, assert `GitError`.
- `TestBranchDelete_missingBranch` — branch doesn't exist, assert `nil` (swallowed).

### `internal/task/finish_test.go`

Seeded fixtures reuse slice-2a helpers. Hermetic where possible; tmux-presence gated where not.

Cases:

- Happy path: clean tree, session alive, `KeepBranches=false` → `RemovedWorktrees` includes both repos, `KilledSession` set, branches deleted, `taskDir` gone.
- Dirty, no `--force` → `WorktreeDirty`, no mutation anywhere.
- Dirty, `--force` → worktree removed, branch deleted with `-D`, session killed.
- One worktree dir already missing → included in `RemovedWorktrees`, other repo still removed.
- `--keep-branches` → branches remain in source repos, `BranchesKept=true`, `taskDir` gone, session killed.
- Task not found → `TaskNotFound`.
- Metadata has no `tmux_session` → `KilledSession=""`, no error.
- Session in metadata but no longer exists at finish time → `KilledSession=""`, no error.
- Idempotent re-run: run once → succeed; run again → `TaskNotFound` (task dir gone).
- tmux unavailable at finish time, metadata has session → `KilledSession=""`, worktrees still removed. (Gated out when tmux is present; hermeticizing requires `PATH` munging, which we skip.)

### `internal/cli/finish_test.go`

- Drive `runFinish(opts)` with a stub `FinishFn` that records params and returns a fixed `FinishResult`.
- Assert envelope shape on piped mode, including `omitempty` for `killed_session`.
- Assert invalid slug → `invalid_args` envelope, exit 2.
- Assert `FinishFn` error propagation for `WorktreeDirty` and `TaskNotFound`.

### What we do not test

- Real tmux under `kill-session` where `--force` is in play — we trust slice-2b's coverage of the primitive.
- `os.RemoveAll` behavior on the task dir — Go stdlib.

## Commit plan

Each commit keeps `go test ./...` green. Conventional commits.

1. `docs: add AGENTS.md codifying repo philosophy + CLAUDE.md symlink`
2. `refactor(cli): extract shared task slug regex`
3. `refactor(cli): extract probeSessionState into shared helper`
4. `refactor(cli): extract tasks-dir scan into scanTasks helper`
5. `feat(git): add WorktreeRemove, WorktreeDirty, BranchDelete primitives`
6. `feat(task): add Finish orchestrator`
7. `feat(cli): add ds finish command`

PR opens after commit 7. PR body references v0.2 milestone, PR #4, and this spec.

## File tree delta

```
AGENTS.md                              # new
CLAUDE.md                              # new (symlink -> AGENTS.md)
internal/
  git/
    branch.go                          # edit: + BranchDelete
    branch_test.go                     # edit: + BranchDelete cases
    worktree.go                        # edit: + WorktreeRemove, WorktreeDirty
    worktree_test.go                   # edit: + new cases
  task/
    finish.go                          # new
    finish_test.go                     # new
  cli/
    slug.go                            # new: shared taskSlugRe
    tmux_state.go                      # new: shared probeSessionState
    tasks_scan.go                      # new: shared scanTasks
    new.go                             # edit: drop local slugRe
    attach.go                          # edit: drop attachSlugRe
    list.go                            # edit: use probeSessionState + scanTasks
    pick.go                            # edit: use probeSessionState + scanTasks
    finish.go                          # new
    finish_test.go                     # new
    root.go                            # edit: register finish
docs/superpowers/specs/
  2026-04-24-v0-2-slice-1-design.md    # this file
docs/superpowers/plans/
  2026-04-24-v0-2-slice-1.md           # follows via writing-plans
```

## Open questions

None blocking slice 1. Still tracked as v0.2 candidates:

- Derive-from-filesystem metadata model — carried over from slice-2b's open questions. Touches slice-2a code.
- Narrower tmux error codes at callsites — needs a new generic `tmux_error` code in SPEC's enum; additive whenever it lands.
- Post-create hooks (SPEC mentions v0.2; deliberately not touched here).
