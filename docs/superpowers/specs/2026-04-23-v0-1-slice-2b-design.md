# v0.1 Slice 2b — `ds pick` + `ds attach` (+ PR #3 carry-overs)

**Status:** approved 2026-04-23
**Scope:** this doc covers slice 2b only. Slice 2a (`ds new`, `ds list`, core packages) shipped in PR #3, merge `77461d6`.
**Prior art:** `docs/superpowers/specs/2026-04-23-v0-1-slice-2a-design.md`, `docs/superpowers/plans/2026-04-23-v0-1-slice-2a.md`. Canonical spec: `SPEC.md` v0.5.

## Goal

Ship the two resume-path task commands from `SPEC.md`:

- `ds pick` — `fzf` over tasks discovered on disk, preview of repos / branch / ticket / session state. On TTY, Enter delegates to `ds attach <slug>`. Piped, emits an envelope carrying the selected task.
- `ds attach <slug>` — attaches to the task's tmux session. If the session died, recreate it from `.task.json` before attaching. On TTY, exec replaces the process with `tmux attach`. Piped, emits an envelope with `{slug, session, was_recreated, attach_command}`.

Slice 2b closes the v0.1 milestone's command set except for `finish` + `create-interactive` (v0.2).

## Non-goals for this slice

- `ds finish`, `ds create-interactive` — v0.2 per SPEC.
- Reworking the `.task.json` metadata model. Pushback surfaced during brainstorming: most of the persisted fields (repos, branches, worktree paths) are derivable from the filesystem + tmux; only `ticket`, `ticket_url`, and `base` are irreducible. That refactor belongs in its own slice (v0.2 candidate) and would touch slice-2a code. For 2b we treat `.task.json` as authoritative, consistent with slice-2a.
- Migrating the on-disk layout to `.ds/` or `.ds.json`. Same reasoning: it's a separate pass.
- Persisting fzf selection history, styling customization, multi-select, or preview format knobs.
- Partial-failure rollback on attach-recreate. Failures leave whatever tmux objects the attempt created; the user re-runs.
- `schema_version` on envelopes. Deferred to v0.2 per SPEC.

## PR #3 carry-overs

Land as the first two commits on the slice-2b branch so downstream commits benefit.

### Carry-over 1: `internal/tmux.wrapTmuxErr` code parameter

`wrapTmuxErr` hardcodes `errs.TmuxUnavailable` for every wrapped failure. `ds attach`'s recreate path needs to emit `tmux_session_not_found` for a specific case, and the label "unavailable" is also wrong for, say, `new-session` against a running-but-full tmux.

New signature:

```go
func wrapTmuxErr(code errs.Code, op string, err error, stderr []byte) error
```

`run(args ...)` becomes `run(code errs.Code, args ...)` so the caller specifies the narrower code at the callsite. Existing callers migrate as follows:

| callsite | code |
|---|---|
| `Available` | `TmuxUnavailable` |
| `HasSession` (hard error path, not exit-1) | `TmuxUnavailable` |
| `IsSessionAttached` | `TmuxUnavailable` |
| `NewSession` | `TmuxUnavailable` |
| `NewWindow` | `TmuxUnavailable` |
| `KillSession` | `TmuxUnavailable` |

Every existing callsite keeps `TmuxUnavailable` in v0.1 — no user-visible change — but slice 2b's `ds attach` recreate gains the ability to construct a `TmuxSessionMissing` error when metadata records no session and the user didn't pass `--start-tmux`.

### Carry-over 2: `internal/git.WorktreeAdd` classifies "already exists"

`git worktree add` fails with stderr like `fatal: '<path>' already exists` when the target path is occupied, and with different stderr when the branch already has a worktree. SPEC exit 5 (`worktree_exists`) is the right code for "path collision"; other git failures remain exit 7 (`git_error`).

Implementation: in `runGit`, after capturing stderr, check `strings.Contains(stderr, "already exists")`. On match, emit `WorktreeExists` with `details: {repo, cmd, git_stderr}`. Everything else stays `GitError`. Substring match is intentionally cheap; `git` stderr for this case is stable across modern git versions.

Only `WorktreeAdd` needs the narrower classification; other `runGit` consumers (`BranchExistsLocal`, `BranchExistsRemote`, `FetchBranch`) never hit that stderr. Simplest implementation threads a `code errs.Code` default into `runGit` the same way the tmux helper does, but a shorter path is: `WorktreeAdd` wraps the `runGit` error, re-codes if the stderr matches. Either works; pick the smaller diff at implementation time.

## New error codes

Added to `internal/errs`:

```go
PickUnavailable   Code = "pick_unavailable"    // exit 2
WorktreeMissing   Code = "worktree_missing"    // exit 5
```

`ExitCode()` gets the two new entries. Rationale:

- `pick_unavailable` (exit 2): `fzf` missing on PATH. Matches the usage/invocation failure family — the caller invoked a command whose required dep isn't present. Distinct stable code so skills can pattern-match and suggest an install.
- `worktree_missing` (exit 5): `.task.json` references a worktree directory that's no longer on disk. Same exit family as `worktree_exists` (collision) — both are worktree-state errors. Agents distinguish by `code`.

Both additions are additive per SPEC's v0.1 error-code rules.

## `internal/task` — `Attach` orchestrator

Mirrors the shape of `Create`. Owns the sequencing of metadata load → session resolution → tmux checks → recreate. Does not know about envelopes or cobra.

```go
type AttachParams struct {
    Slug       string
    TasksDir   string
    TmuxPrefix string
    StartTmux  bool              // --start-tmux; fabricate session when metadata has none
    Now        func() time.Time  // optional; defaults to time.Now
}

type AttachResult struct {
    Task         *Task
    SessionName  string
    WasRecreated bool
}

func Attach(p AttachParams) (*AttachResult, error)
```

Flow:

1. `Load(filepath.Join(p.TasksDir, p.Slug))` — propagate `TaskNotFound` on miss.
2. Resolve session name:
   - `t.TmuxSession != ""` → use it.
   - Else `p.StartTmux` → fabricate `p.TmuxPrefix + p.Slug`, remember to persist after success.
   - Else → `TmuxSessionMissing` with `details: {slug, hint: "pass --start-tmux to create a session for this task"}`, exit 6 (slice-2a already maps `TmuxSessionMissing` into the tmux-error family at `internal/errs/errs.go`; SPEC's enum grouping supports either 6 or 9 for this code, and consistency with the existing mapping is the right tie-breaker).
3. `tmux.Available()` — propagate `TmuxUnavailable` on miss.
4. `tmux.HasSession(name)` — on hit, return `{Task, SessionName: name, WasRecreated: false}`.
5. Recreate preflight: for each `t.Repos[i]`, stat `filepath.Join(taskDir, repo.Worktree)`. First miss → `WorktreeMissing` with `details: {repo, path}`, exit 5. Done before any tmux mutation.
6. `tmux.NewSession(name, t.Repos[0].Name, filepath.Join(taskDir, t.Repos[0].Worktree))`; then `tmux.NewWindow(name, t.Repos[i].Name, <path>)` for `i >= 1`.
7. If the session was fabricated via `--start-tmux`, update `t.TmuxSession = name` and `Write(taskDir, t)` so future invocations see it.
8. Return `{Task: t, SessionName: name, WasRecreated: true}`.

`Attach` is pure orchestration; it does not exec tmux. Exec lives in the cli layer so tests can stub it.

## Command: `ds attach`

### Signature

```
ds attach <slug> [--start-tmux]
```

### Envelope `data` (success)

```json
{
  "slug": "infra-6700-auth-refactor",
  "session": "task-infra-6700-auth-refactor",
  "was_recreated": false,
  "attach_command": "tmux attach -t task-infra-6700-auth-refactor"
}
```

`attach_command` is additive to SPEC's `{slug, session, was_recreated}` — useful for piped consumers that want to render the literal command. Non-breaking.

### TTY vs piped

- **TTY (mode = human):** after `task.Attach` returns success, call the injectable `execFn("tmux", []string{"tmux", "attach", "-t", session}, os.Environ())`. Default `execFn = syscall.Exec`. Process is replaced; no envelope emitted. If exec fails (rare — bad PATH, etc.), fall through to an envelope error.
- **Piped (mode = json):** emit envelope, do not exec.

### Error paths

| condition | code | exit |
|---|---|---|
| slug fails validation | `invalid_args` | 2 |
| `<tasks_dir>/<slug>/.task.json` missing or unreadable | `task_not_found` | 9 |
| metadata has no `tmux_session` and `--start-tmux` not passed | `tmux_session_not_found` | 6 |
| tmux binary missing | `tmux_unavailable` | 6 |
| any worktree dir referenced in metadata is missing on disk | `worktree_missing` | 5 |
| tmux op fails mid-recreate | `tmux_unavailable` (until tmux helper narrows further) | 6 |
| config load failure | `config_error` | 8 |

### `--start-tmux` semantics

- No effect when `t.TmuxSession` is already set.
- When `t.TmuxSession == ""`, fabricate `TmuxPrefix + Slug`, create session + windows, **persist** the name back into `.task.json`. Subsequent `ds attach` / `ds list` see the session without needing the flag again.
- Rejects nothing new — validation identical to the no-flag path.

## Command: `ds pick`

### Signature

```
ds pick
ds pick --preview <slug>        # hidden; fzf preview helper
```

`--preview` is intentionally not advertised in `--help`. Set via `cmd.Flags().MarkHidden("preview")` (or equivalent).

### Flow (no `--preview`)

1. Load config. Walk `<cfg.TasksDir>/*/.task.json`, reuse `task.Load`. Build `[]task.Task` (full metadata for the preview callback to read back on demand).
2. Empty → `TaskNotFound` "no tasks in <tasks_dir>", exit 9.
3. `exec.LookPath("fzf")` — missing → `PickUnavailable`, exit 2, `details: {binary: "fzf"}`.
4. Run the injectable `selectorFn([]string)` with slugs as input. Default implementation: spawn `fzf --height=40% --reverse --no-sort --prompt='task> ' --preview='<argv0> pick --preview {}' --preview-window=right:50%`. fzf uses `/dev/tty` for UI even when our stdout is piped. `<argv0>` is `os.Args[0]` so the preview re-enters the same binary regardless of install path.
5. Selector returns:
   - Selected slug, nil error → proceed.
   - Empty slug, `ErrPickCancelled` → cancellation (Esc / Ctrl-C / non-zero fzf exit).
     - TTY: exit 1, stderr "selection cancelled". No envelope.
     - Piped: emit envelope with `invalid_args` "selection cancelled", exit 2.
   - Error from the selector itself (fzf failed to start, etc.) → bubble as `internal_error`, exit 1.
6. Build per-selection envelope data by re-reading the selected task + tmux state (single call, not the whole list):

```json
{
  "slug": "...",
  "branch": "...",
  "ticket": "...",
  "repos": ["api", "web"],
  "session": "task-...",
  "session_state": "detached",
  "attach_command": "tmux attach -t task-..."
}
```

7. **TTY:** exec self as `ds attach <slug>` via `execFn(argv0, []string{argv0, "attach", slug}, os.Environ())`. Delegates recreate-if-dead + exec-tmux to one place. Note: SPEC says "on Enter, exec tmux attach"; we preserve the user-visible outcome (tmux takes over) while routing through `ds attach` for session-died robustness. Documented divergence.
8. **Piped:** emit envelope from step 6, no exec.

### Flow (`--preview <slug>`)

1. Load config. `task.Load(filepath.Join(cfg.TasksDir, slug))`.
2. If load fails, print a one-line stderr error and exit 0 (fzf calls preview constantly; hard-failing its preview kills the whole picker).
3. Print a plain-text block to stdout — no envelope, no JSON. Fixed layout:

```
slug:    infra-6700-auth-refactor
type:    feat
ticket:  INFRA-6700
branch:  feat/infra-6700-auth-refactor
base:    main
repos:   api, web
session: task-infra-6700-auth-refactor (detached)
```

`session` line elides when `tmux_session == ""`. Session state probed live via `tmux.HasSession` / `IsSessionAttached`, falling back to `absent` if tmux isn't on PATH.

### Error paths (primary command)

| condition | code | exit |
|---|---|---|
| `fzf` missing | `pick_unavailable` | 2 |
| `<tasks_dir>` empty / no `.task.json` under it | `task_not_found` | 9 |
| selection cancelled | generic on TTY; `invalid_args` envelope if piped | 1 / 2 |
| config load failure | `config_error` | 8 |

## Output layering

Same shape as slice 2a:

- `NewAttachCmd(flags *GlobalFlags)` / `NewPickCmd(flags *GlobalFlags)`; package-private `runAttach(opts)` and `runPick(opts)`.
- `RunE` wraps non-zero `runX` into `&ExitError{code}`.
- `cmd.OutOrStdout()` / `cmd.ErrOrStderr()` threaded into opts.
- Opts include `ExecFn` (default `syscall.Exec`) and `LookupFn` (default `exec.LookPath`) for testability.
- `pickOpts` additionally carries `SelectorFn` so tests don't need `fzf` installed.

## Testing strategy

### `internal/task/attach_test.go`

- Seeded fixture repos (reuse slice-2a helpers).
- Cases, all hermetic where possible, tmux-presence gated where not:
  - Session alive → `WasRecreated=false`, no mutation.
  - Session missing, recreate OK → `WasRecreated=true`, session created, windows per repo.
  - Session missing, one worktree dir deleted → `WorktreeMissing` error, tmux untouched.
  - Metadata has no session, no `--start-tmux` → `TmuxSessionMissing`.
  - Metadata has no session, `--start-tmux` → session created, `.task.json` rewritten with new `tmux_session`.
- All tmux-using cases `t.Setenv("TMUX_TMPDIR", t.TempDir())` and `t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })`.

### `internal/cli/attach_test.go`

- Drive `runAttach(opts)` with a stub `ExecFn` that records `(argv0, argv, env)` and returns nil.
- Assert envelope shape on the piped path; assert exec args on the TTY path; assert no exec on error paths.
- Gate tmux-touching subtests on `exec.LookPath("tmux")`.

### `internal/cli/pick_test.go`

- **Preview path:** direct test on `runPickPreview(opts, slug)` — seeded task dir, assert stdout block byte-for-byte (small, stable format).
- **Primary path:** inject `SelectorFn` that returns a fixed slug. Stub `ExecFn`. Seed two tasks. Cases:
  - TTY, selector returns `task-a` → ExecFn called with `ds attach task-a`.
  - Piped, selector returns `task-a` → envelope emitted, no exec.
  - TTY, selector returns `ErrPickCancelled` → exit 1, stderr contains "cancelled".
  - Piped, selector returns `ErrPickCancelled` → `invalid_args` envelope, exit 2.
- **fzf-missing:** stub `LookupFn` to return `exec.ErrNotFound` → `pick_unavailable` envelope, exit 2.
- **Empty tasks dir:** seeded empty dir → `task_not_found` envelope, exit 9.

### Carry-overs

- `internal/tmux/session_test.go`: update existing tests to match new `wrapTmuxErr` signature. Add a case asserting the error's `Code` matches the one passed in by the caller (regression guard).
- `internal/git/worktree_test.go`: add a case where a pre-existing dir at `worktreePath` causes `git worktree add` to emit "already exists"; assert `err.Code == WorktreeExists`. Existing cases continue to assert `GitError`.

### What we do not test

- Real interactive `fzf` — gated out via `SelectorFn` injection. No CI flakiness from a missing-binary or TTY-missing runner.
- Actual `syscall.Exec` replacement — we assert the argv we would have passed, not that the process got replaced. Functional verification belongs to manual smoke testing post-merge.

## Commit plan

Each commit keeps `go test ./...` green. Conventional commits.

1. `refactor(tmux): parameterize wrapTmuxErr with explicit error code`
2. `refactor(git): map "already exists" stderr to worktree_exists`
3. `feat(errs): add pick_unavailable and worktree_missing codes`
4. `feat(task): add Attach orchestrator with recreate + start-tmux`
5. `feat(cli): add ds attach`
6. `feat(cli): add ds pick with hidden --preview`

PR opens after commit 6. PR body references v0.1 milestone + PR #3 and carry-over review comments.

## File tree delta

```
internal/
  errs/
    errs.go                        # edit: 2 new codes + ExitCode mapping
    errs_test.go                   # edit: exit-code table additions
  tmux/
    session.go                     # edit: wrapTmuxErr signature
    session_test.go                # edit
  git/
    worktree.go                    # edit: classify "already exists"
    worktree_test.go               # edit
  task/
    attach.go                      # new
    attach_test.go                 # new
  cli/
    attach.go                      # new
    attach_test.go                 # new
    pick.go                        # new
    pick_test.go                   # new
    root.go                        # edit: register attach + pick
```

## Open questions

None blocking 2b. For the v0.2 successor slice:

- Derive-from-filesystem metadata model — move `repos`, `branches`, `worktree paths` out of `.task.json` and compute live. Keep only `ticket`, `ticket_url`, `base`, `type` in a sidecar.
- Move the sidecar to `.ds/task.json` once `finish` adds more persisted state (logs, cleanup log, hooks).
- Narrower tmux error codes — `wrapTmuxErr` now takes a code parameter but we still funnel most runtime failures through `TmuxUnavailable` because the v0.1 enum lacks a generic `tmux_error`. Adding one is additive.
