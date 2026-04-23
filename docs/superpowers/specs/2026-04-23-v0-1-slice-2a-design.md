# v0.1 Slice 2a — `ds new` + `ds list` (+ PR #2 carry-overs)

**Status:** approved 2026-04-23
**Scope:** this doc covers slice 2a only. Slice 2b (`ds pick` + `ds attach`) gets its own design.
**Prior art:** `docs/superpowers/plans/2026-04-23-v0-1-foundation.md` (slice 1, shipped in PR #2, merge `c9d885a`). Canonical spec: `SPEC.md` v0.5.

## Goal

Ship the two disk-only task commands from `SPEC.md`:

- `ds new <slug> --repos <r1,r2,...> [flags]` — creates `<tasks_dir>/<slug>/` with one git worktree per repo, writes `.task.json`, optionally opens a tmux session with one window per repo.
- `ds list` — globs `<tasks_dir>/*/.task.json`, emits the envelope documented in `SPEC.md`, reports per-task tmux session state.

`ds pick` and `ds attach` are deferred to slice 2b; they rely on a live tmux for their primary value (picker → attach, attach → exec). Building disk-only commands first gives us an end-to-end loop (`new` writes → `list` reads) and isolates the fzf/tmux-heavy attach flow into its own review.

## Non-goals for this slice

- `ds finish`, `ds create-interactive`, `ds pick`, `ds attach` — later slices.
- Partial-failure rollback on `ds new`. Spec has `ds finish` for cleanup in v0.2. v0.1 leaves partial state on disk and the user cleans up manually.
- Base-branch freshness. We do not `git fetch` the base before worktree creation. Users manage that themselves. Future `--fetch-base` flag possible.
- tmux session reuse. If a session with the target name already exists, `ds new` errors; `ds attach` (slice 2b) handles resume.
- `schema_version` on envelopes. Deferred to v0.2 per SPEC.

## PR #2 carry-overs

Land as the first commit on the slice-2a branch so they are out of the way before any new packages appear:

1. **Typed `ReposData` / `RepoItem`.** `internal/cli/repos.go::marshalReposData` currently returns `map[string]any`. Replace with:

   ```go
   type ReposData struct {
       Repos []RepoItem `json:"repos"`
       Roots []string   `json:"roots"`
   }
   type RepoItem struct {
       Name string `json:"name"`
       Path string `json:"path"`
       Root string `json:"root"`
   }
   ```

   Update `TestRepos_successGoldenEnvelope` to `json.Unmarshal` into a struct that embeds `ReposData` inside an envelope, and assert on typed fields instead of `map[string]any` probing.

2. **Thread `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`.** `NewReposCmd`'s RunE hardcodes `os.Stdout` / `os.Stderr` when populating `reposOpts`. Swap to `cmd.OutOrStdout()` / `cmd.ErrOrStderr()` so cobra `Execute`-based integration tests (e.g. `cmd.SetOut(&buf)`) can observe output. Existing unit tests drive `runRepos` directly and stay unaffected.

## Packages (new)

### `internal/git`

Pure wrapper over `git`. No state, no caching. Every function shells out to `git` with an explicit `-C <repoPath>` so callers never need to `chdir`. Stderr from failing git invocations is captured into `TaskError.Details["git_stderr"]`.

```go
type AddMode int
const (
    CreateFromBase     AddMode = iota // git worktree add -b <branch> <path> <base>
    CheckoutExisting                   // git worktree add <path> <branch>
    FetchAndTrack                      // git fetch + git worktree add --track -b <branch> <path> origin/<branch>
)

func WorktreeAdd(repoPath, worktreePath, branch, base string, mode AddMode) error
func BranchExistsLocal(repoPath, branch string) (bool, error)
func BranchExistsRemote(repoPath, remote, branch string) (bool, error) // uses `git ls-remote --exit-code`
func FetchBranch(repoPath, remote, branch string) error                 // `git fetch <remote> <branch>`
```

Mode is decided by the caller (in `internal/task`). `WorktreeAdd` does not decide; it just runs the right `git worktree add` form for the mode. This decomplects branch-resolution logic from worktree creation.

Errors returned by these helpers are `*TaskError{Code: GitError}` with details carrying `cmd`, `stderr`, `repo`. Callers wrap as needed.

### `internal/tmux`

Thin wrapper over `tmux`. All commands run against the default socket unless `TMUX_TMPDIR` points elsewhere (tests use a dedicated socket via `TMUX_TMPDIR` override or `-L` flag — implementation detail; public API takes no socket argument).

```go
func Available() error // TaskError{TmuxUnavailable} if `tmux` not on PATH
func HasSession(name string) (bool, error)
func IsSessionAttached(name string) (bool, error) // tmux display -p -t <name> "#{session_attached}"
func NewSession(name, firstWindowName, cwd string) error
func NewWindow(session, name, cwd string) error
func KillSession(name string) error
```

`NewSession` runs `tmux new-session -d -s <name> -n <firstWindow> -c <cwd>` so sessions stay detached until an explicit `attach` (spec says `ds new` stays detached; `ds attach` handles foreground in slice 2b).

`NewWindow` runs `tmux new-window -t <session> -n <name> -c <cwd>`.

`KillSession` wraps `tmux kill-session -t <name>`. No-op if session missing — callers checking first is fine.

### `internal/task`

Owns `.task.json` round-trip and the `ds new` orchestration. Does not know about envelopes or cobra.

```go
type Task struct {
    Slug        string    `json:"slug"`
    Type        string    `json:"type"`
    Ticket      string    `json:"ticket,omitempty"`
    TicketURL   string    `json:"ticket_url,omitempty"`
    Branch      string    `json:"branch"`
    Base        string    `json:"base"`
    Repos       []RepoRef `json:"repos"`
    TmuxSession string    `json:"tmux_session,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
}

type RepoRef struct {
    Name     string `json:"name"`
    Path     string `json:"path"`      // absolute path to source repo
    Worktree string `json:"worktree"`  // directory name under <tasks_dir>/<slug>/
}

type CreateParams struct {
    Slug       string
    Type       string
    Ticket     string
    TicketURL  string
    BranchOverride string // --branch wins over derivation
    Base       string
    TasksDir   string
    Repos      []ResolvedRepo // name + abs source path, already resolved from --repos
    NoTmux     bool
    StartTmux  bool  // from config
    Strict     bool
    TmuxPrefix string
    Now        func() time.Time // injected for tests
}

type ResolvedRepo struct {
    Name string
    Path string // absolute source repo path
}

type CreateResult struct {
    Task        *Task
    RepoStates  []RepoState // index-aligned with params.Repos
    TaskDir     string
}

type RepoState struct {
    Name         string
    WorktreePath string
    BranchState  string // "created" | "checked_out_existing" | "fetched_tracking"
}

func Create(p CreateParams) (*CreateResult, error)
func Load(taskDir string) (*Task, error)
func Write(taskDir string, t *Task) error
```

`Create` orchestrates:

1. Derive branch name (`BranchOverride` wins; else `{type}/[{ticket-lower}-]{slug}`).
2. `os.Stat(<tasksDir>/<slug>)` → `TaskError{TaskExists}` if present.
3. `os.MkdirAll(taskDir, 0o755)`.
4. Per repo: pick `AddMode` (local exists → `CheckoutExisting`; else remote exists → `FetchAndTrack`; else `CreateFromBase`). `Strict` + non-`CreateFromBase` → `TaskError{BranchConflict}`.
5. `git.WorktreeAdd(repo.Path, filepath.Join(taskDir, repo.Name), branch, base, mode)` per repo.
6. If `StartTmux && !NoTmux`: session name = `TmuxPrefix + Slug`. `tmux.HasSession` → error `TmuxSessionExists` if already present. Else `NewSession` with first window for repo[0], then `NewWindow` for repo[1..].
7. `Write(taskDir, task)`.

Partial failure: if step 5 fails on repo N, steps 1-4 and partial worktrees persist. `.task.json` is not written (step 7). User cleans up manually or with `ds finish` in v0.2. Documented behavior; no rollback in v0.1.

## Command: `ds new`

### Flags (cobra)

```
Args: exactly 1 positional (slug)
--repos <csv>         required
--type <t>            default from config (feat)
--ticket <id>
--branch <b>          override derived branch entirely
--base <b>            default from config (main)
--no-tmux
--strict
```

Global `--json` / `--human` inherited from root.

### Validation / resolution

- Slug regex: `^[a-z0-9][a-z0-9._-]*$` (case-insensitive not allowed — keep task dirs predictable). Violation → `invalid_args`.
- `--type` must be in `{feat, fix, chore, refactor, docs, test, perf, build, ci}`. Violation → `invalid_args`.
- `--repos` empty → `invalid_args`.
- Load config. Walk `cfg.RepoRoots` via `discover.Walk`. For each name in `--repos`: match against `discover.Repo.Name`. Miss → `TaskError{RepoNotFound}` with `details: {"requested": "<name>", "available": [...]}`. Duplicate-name disambiguation per spec: second collision is reported as `<root-basename>/<repo-name>` by `discover`.
- Derive branch. Validate `--branch` is non-empty if set.
- Build `CreateParams`, call `task.Create`.

### Envelope `data` (success)

Per SPEC:

```json
{
  "slug": "...",
  "path": "<tasks_dir>/<slug>",
  "branch": "feat/...",
  "base": "main",
  "ticket": "INFRA-6700",
  "repos": [
    {"name": "api", "worktree_path": "...", "branch_state": "created"}
  ],
  "tmux_session": "task-<slug>",
  "attach_command": "tmux attach -t task-<slug>"
}
```

When `--no-tmux` or `start_tmux: false`: omit `tmux_session`, omit `attach_command`. (SPEC shows them always populated; we omit with `json:",omitempty"` since no session was created. Additive change, not breaking.)

### Error paths (concrete)

| Condition | Code | Exit |
|---|---|---|
| bad slug / type / empty `--repos` | `invalid_args` | 2 |
| requested repo not in discovery | `repo_not_found` | 3 |
| `<tasks_dir>/<slug>` already exists | `task_exists` | 4 |
| `--strict` + branch exists (local or remote) | `branch_conflict` | 5 |
| git worktree add fails (path collision or else) | `worktree_exists` or `git_error` | 5 / 7 |
| any other git failure | `git_error` | 7 |
| tmux binary missing | `tmux_unavailable` | 6 |
| session with target name already present | `tmux_session_exists` | 6 |
| config load failure | `config_error` | 8 |

`worktree_exists` vs `git_error`: if `git worktree add` fails and stderr matches `already exists`, emit `worktree_exists`; otherwise `git_error`. Cheap string match acceptable — `git` stderr shape is stable enough for v0.1.

## Command: `ds list`

### Flags

None beyond global `--json` / `--human`.

### Flow

1. Load config (for `TasksDir` and `TmuxPrefix`).
2. `filepath.Glob(filepath.Join(tasksDir, "*", ".task.json"))`.
3. For each match:
   - `task.Load`. On decode error: emit one `warn: <path>: <err>` line to stderr, skip the entry. Never hard-error.
   - Derive session state: `tmux.HasSession(task.TmuxSession)` → `absent` if no session or `task.TmuxSession == ""`; else `tmux.IsSessionAttached` → `attached` / `detached`.
4. Build `data.tasks` array per SPEC:

```json
{
  "tasks": [
    {
      "slug": "...",
      "type": "feat",
      "ticket": "INFRA-6700",
      "branch": "...",
      "repos": ["api", "web"],
      "session": "task-...",
      "session_state": "detached",
      "created_at": "..."
    }
  ]
}
```

`tasks` is `[]` when `tasksDir` is empty. Never nil in JSON.

When `tmux` binary is missing: `session_state` defaults to `absent` for every task, no hard error. `ds list` must work on a box without tmux installed.

### Error paths

| Condition | Code | Exit |
|---|---|---|
| config load failure | `config_error` | 8 |
| `tasksDir` missing on disk | success, empty list | 0 |
| individual `.task.json` decode error | warn-and-skip | 0 |

## Output layering

Keep the slice-1 shape:

- Each command file has `NewXCmd(flags *GlobalFlags) *cobra.Command` and a package-private `runX(opts) int`.
- `RunE` wraps `runX` in `&ExitError{code}` on non-zero.
- `runX` takes explicit `Stdout`, `Stderr`, `Mode`, and any other injectables (`ConfigPath`, `Now`, etc).
- Cmd layer threads `cmd.OutOrStdout()` / `cmd.ErrOrStderr()` into opts (matches fix from carry-over #2 — applied uniformly to new commands from day one).

## Testing strategy

### Fixture git repos

Helper in `internal/git/testhelpers_test.go` (or shared `internal/testutil` if the helper is reused):

```go
func initFixtureRepo(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()
    run(t, dir, "git", "init", "-b", "main")
    run(t, dir, "git", "config", "user.email", "test@example.com")
    run(t, dir, "git", "config", "user.name", "test")
    write(t, dir, "README.md", "x")
    run(t, dir, "git", "add", ".")
    run(t, dir, "git", "commit", "-m", "init")
    return dir
}
```

Return absolute path; callers drive `WorktreeAdd` etc against it.

For remote-branch scenarios, create a second fixture repo and `git clone` or `git remote add`, then fetch.

### tmux tests

- Gate via `exec.LookPath("tmux")` inside each test: `t.Skip("tmux not on PATH")` if missing.
- Public API in `internal/tmux` takes no socket argument. Tests set `TMUX_TMPDIR=t.TempDir()` via `t.Setenv` so each test gets its own socket directory and auto-cleanup.
- `t.Cleanup` calls `tmux kill-server` to tear down the per-test server cleanly.

### Package tests

- `internal/git`: one test per `AddMode`, plus not-found cases. `BranchExistsRemote` wires up a local bare repo as `origin`.
- `internal/tmux`: basic round-trip (`NewSession` → `HasSession == true` → `KillSession` → `HasSession == false`). `Available` test stubs PATH.
- `internal/task`: `Create` → `Write` → `Load` round-trip on fixture repos. `--no-tmux` path tested end-to-end. Tmux path gated on binary.

### CLI tests

- `internal/cli/new`: drive `runNew(opts)` with fixture repos and `NoTmux: true` to keep hermetic. Assert envelope JSON. One gated test exercises the tmux path.
- `internal/cli/list`: seed `tasksDir` with hand-written `.task.json` files; assert envelope. Include a malformed file case (expect skip + stderr warning).

### What we do NOT test

- Actual `exec tmux attach` fg path — that's slice 2b.
- Cross-platform behavior. Linux + macOS only, spec already says so.
- Pre-existing-but-unrelated tmux windows. Session collisions are tested at the `HasSession` → error level.

## Commit plan

Each commit keeps `go test ./...` green. Conventional commits.

1. `refactor(cli): typed ReposData and use cmd io writers in repos`
2. `feat(git): worktree + branch helpers`
3. `feat(tmux): session + window helpers`
4. `feat(task): Task struct + Create + metadata io`
5. `feat(cli): add ds new`
6. `feat(cli): add ds list`

PR opens after commit 6. PR body references v0.1 milestone + slice-1 PR (#2) and carry-over review comments.

## File tree delta

```
internal/
  git/
    worktree.go
    branch.go
    worktree_test.go
    branch_test.go
  tmux/
    session.go
    session_test.go
  task/
    task.go        # struct + Load/Write
    create.go      # Create orchestrator
    task_test.go
    create_test.go
  cli/
    new.go
    new_test.go
    list.go
    list_test.go
    repos.go       # edited (carry-over)
    repos_test.go  # edited (carry-over)
    root.go        # edited: register new + list
```

## Open questions

None blocking slice 2a. Slice 2b will decide:

- tmux socket strategy for `pick` + `attach` tests.
- `ds pick` behavior when `fzf` binary missing (error with `invalid_args`? new `pick_unavailable` code? probably just `internal_error` with a clear message).
- `ds attach` recreation semantics when `.task.json` references a worktree that's since been deleted — error or recover silently?

These do not block 2a.
