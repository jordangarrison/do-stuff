# do-stuff: Task Tooling Spec (v0.5)

An opinionated, minimal toolkit for spinning up and resuming multi-repo work organized as "tasks." Three layers (core library, CLI, skills), each thin and independently useful. Distributed as a single repo containing both the CLI binary and the Claude skills that drive it.

## Goals

- One predictable directory layout per task, with a worktree per repo.
- Fast, LLM-free resume from desktop hotkey or SSH.
- Claude-driven task creation from a Jira ticket when LLM judgment helps.
- Extensible via a router skill as new task-shaped workflows appear.
- Agent-first output contract: every CLI response is structured by default.
- Installable as a unit via `npx skills add github.com/jordangarrison/do-stuff`.

## Non-goals

- Agent status dashboards, parallel-agent coordination, live pane preview. (Orchard's territory.)
- Cross-platform support beyond Linux/macOS.
- Replacing Grove, workmux, or similar tools. This is complementary.

## Distribution

The project is a single public GitHub repo, `github.com/jordangarrison/do-stuff`, containing:

```
do-stuff/
  cmd/ds/        # Go CLI main package
  internal/            # Go internal packages
  skills/              # Claude skills (router + leaves)
    SKILL.md           # router
    task-new/
      SKILL.md
    task-finish/
      SKILL.md
  install.sh           # installs binary + skills from a local checkout
  README.md
  flake.nix            # Nix users get a derivation for free
  go.mod
```

Install paths, v0.1:

- **Nix users:** `nix profile install github:jordangarrison/do-stuff` installs the binary. Skills install via the home-manager module in v0.2; for v0.1, Nix users copy `skills/` manually or clone the repo and symlink.
- **Any OS, agent-led install:** `npx skills add github.com/jordangarrison/do-stuff` fetches the repo, runs `install.sh`, places the binary on PATH, and copies `skills/` into the user's Claude skills directory. This is the primary path for users who want both CLI and skills in one step.
- **Go-only, no skills:** `go install github.com/jordangarrison/do-stuff/cmd/ds@latest` builds the binary from source. No skills installed; user adds them manually if they want them.

For Jordan's setup specifically: the Nix path is primary (flake input in home-manager), with skills installed by the same home-manager config in v0.2. `npx skills add` is for anyone else who wants the tool without committing to Nix.

## Decisions (locked for v0.1)

- **Name:** `do-stuff` is the project, repo, Go module, config directory, and default tasks directory. `ds` is the CLI binary, chosen for fast invocation from hotkeys and SSH prompts. Similar to how `ripgrep` ships `rg`, or `fd-find` ships `fd`.
- **Language:** Go with Cobra. Single static binary.
- **Default task directory:** `~/.do-stuff/<slug>/` (user-overridable).
- **Repo roots:** user-configured; no default that assumes `~/dev`.
- **Repo discovery:** walk roots on demand every invocation.
- **Branch template:** `{type}/[{ticket-lower}-]{slug}`
    - `{type}` ∈ `feat|fix|chore|refactor|docs|test|perf|build|ci`
    - `{ticket-lower}` only when `--ticket` passed.
- **Branch reuse:** if the branch exists (local or remote), check it out into the worktree. `--strict` forces failure on existing branches.
- **Type selection:** skill infers from ticket; interactive CLI prompts; non-interactive CLI takes `--type` (default `feat`).
- **Output mode:** `isatty(stdout)` auto-detect. `--json` / `--human` override.
- **Output shape:** envelope with tagged error codes (see "Output contract").
- **tmux layout:** one window per repo, each named after the repo, cwd at the worktree, plain shell. No auto-run commands in v0.1.
- **Bootstrap strategy:** do nothing in v0.1; rely on direnv / devenv / whatever the user has in-repo to bootstrap on first `cd`. Post-create hooks land in v0.2.

## Architecture

Three layers, bottom up. Each layer calls only the layer beneath it.

```
+-----------------------------------------------------+
|   Skills layer (LLM judgment)                       |
|   router -> task-new, task-finish, (future)         |
+-----------------------------------------------------+
|   CLI layer (Go / Cobra, agent-first JSON)          |
|   ds new, pick, attach, list, repos, finish,        |
|   create-interactive                                |
+-----------------------------------------------------+
|   Core library (Go packages, deterministic)         |
|   internal/task, internal/git, internal/tmux,       |
|   internal/config, internal/discover, internal/errs |
+-----------------------------------------------------+
```

Skills never skip the CLI. A skill's final action is shelling out to the `ds` binary and parsing the JSON envelope. The CLI is the single source of truth for what each operation means.

## Output contract

### Mode detection

- stdout is a TTY → pretty human output on stdout, structured error summary on stderr only.
- stdout is piped or redirected → JSON envelope on stdout, one-line human summary on stderr.
- `--json` forces JSON at a TTY. `--human` forces pretty when piped (rarely useful, included for completeness).

### Envelope

```json
{
  "ok": true,
  "command": "ds.new",
  "data": { ... }
}
```

```json
{
  "ok": false,
  "command": "ds.new",
  "error": {
    "code": "repo_not_found",
    "message": "no repo matching 'api' found in configured roots",
    "details": { "requested": "api", "available": ["dotfiles", "orchard"] }
  }
}
```

Fields:

- `ok` (bool, required)
- `command` (string, required) — dotted for routing (`ds.new`, `ds.pick`, etc.)
- `data` (object, present iff `ok == true`)
- `error` (object, present iff `ok == false`)

### Schema versioning

v0.1 omits a `schema_version` field. v0.2+ adds one when breaking an existing payload. Additive changes (new fields) are not breaking.

### Exit codes

- `0` — success
- `1` — generic / unexpected error
- `2` — invalid args / usage
- `3` — repo not found
- `4` — task directory exists
- `5` — worktree conflict under `--strict`
- `6` — tmux error
- `7` — git error (not covered above)
- `8` — config error (malformed TOML, bad path)
- `9` — not found (task / branch / session specified by user)

### Error codes (stable, v0.1)

- `invalid_args`
- `repo_not_found`
- `task_exists`
- `task_not_found`
- `branch_conflict`
- `worktree_exists`
- `worktree_dirty`
- `tmux_unavailable`
- `tmux_session_exists`
- `tmux_session_not_found`
- `git_error`
- `config_error`
- `internal_error`

`code` is stable and enumerated. `message` is human-ish and may change. `details` is free-form per code and documented alongside.

## Data model

### Task directory layout

```
<tasks_dir>/<slug>/
  .task.json
  <repo-1>/       # git worktree
  <repo-2>/
  ...
```

Default `tasks_dir` is `~/.do-stuff`.

### `.task.json`

```json
{
  "slug": "infra-6700-auth-refactor",
  "type": "feat",
  "ticket": "INFRA-6700",
  "ticket_url": "https://flocasts.atlassian.net/browse/INFRA-6700",
  "branch": "feat/infra-6700-auth-refactor",
  "base": "main",
  "repos": [
    {"name": "api", "path": "/abs/path/to/api", "worktree": "api"}
  ],
  "tmux_session": "task-infra-6700-auth-refactor",
  "created_at": "2026-04-22T15:04:05Z"
}
```

No central index. `ds list` globs `<tasks_dir>/*/.task.json`.

### Configuration

```
~/.config/do-stuff/config.toml
```

```toml
tasks_dir         = "~/.do-stuff"
repo_roots        = []              # user must set or use --roots
tmux_prefix       = "task-"
default_base      = "main"
default_type      = "feat"
start_tmux        = true
post_create_hooks = []              # v0.2
```

All settings have defaults except `repo_roots`, which the user must configure on first run. First-run UX: if `repo_roots` is empty, the CLI emits a `config_error` with clear `details` on what to set, and the skill proposes a one-liner to write the config.

### Repo discovery

`discover.Walk(roots)` walks each root to depth 2 looking for `.git` directories. Expected cost: single-digit ms for tens of repos. Disambiguation when two roots contain a same-named repo: the second appearance is reported as `<root-basename>/<repo-name>`.

## Go project layout

```
cmd/ds/
  main.go                 # cobra root wiring

internal/
  cli/
    root.go
    new.go
    pick.go
    attach.go
    list.go
    repos.go
    finish.go
    interactive.go
    output.go             # envelope, TTY detection, rendering

  task/
    task.go               # Task struct, metadata I/O
    create.go
    finish.go

  git/
    worktree.go           # git worktree add / remove
    branch.go             # existence, fetch, tracking

  tmux/
    session.go            # new-session, new-window, kill-session

  discover/
    walk.go

  config/
    config.go             # toml load, defaults, validation

  errs/
    errs.go               # TaskError + codes
```

### Error type

```go
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

type TaskError struct {
    Code    Code           `json:"code"`
    Message string         `json:"message"`
    Details map[string]any `json:"details,omitempty"`
}

func (e *TaskError) Error() string { return e.Message }
func (e *TaskError) ExitCode() int { ... }
```

Internal functions return `error`. The CLI boundary recovers the structured form via `errors.As`. Unwrapped errors become `internal_error` with exit code 1 — never silently swallow a real bug as a structured error.

### Output layer

```go
type Envelope struct {
    OK      bool       `json:"ok"`
    Command string     `json:"command"`
    Data    any        `json:"data,omitempty"`
    Error   *TaskError `json:"error,omitempty"`
}

func Render(cmd *cobra.Command, command string, data any, err error) int
```

Cobra `RunE` returns error. `main` calls `Render`, then `os.Exit`.

## CLI commands

All emit envelopes per the contract. Flag parsing via Cobra.

### `ds new <slug> --repos <r1,r2,...> [flags]`

Flags:

- `--type <t>` — default `feat`
- `--ticket <id>` — include in branch and metadata
- `--branch <b>` — override derived branch entirely
- `--base <b>` — base branch (default config, else `main`)
- `--no-tmux`
- `--strict`
- `--json` / `--human`

Success `data`:

```json
{
  "slug": "infra-6700-auth-refactor",
  "path": "/home/jordan/.do-stuff/infra-6700-auth-refactor",
  "branch": "feat/infra-6700-auth-refactor",
  "base": "main",
  "ticket": "INFRA-6700",
  "repos": [
    {"name": "api", "worktree_path": "...", "branch_state": "created"},
    {"name": "web", "worktree_path": "...", "branch_state": "checked_out_existing"}
  ],
  "tmux_session": "task-infra-6700-auth-refactor",
  "attach_command": "tmux attach -t task-infra-6700-auth-refactor"
}
```

`branch_state` ∈ `created`, `checked_out_existing`, `fetched_tracking`.

### `ds pick`

No flags. `fzf` over `ds list`, preview shows repos / branch / ticket.

- TTY: on Enter, `exec tmux attach`.
- Piped: print envelope of selected task, no attach.

### `ds attach <slug>`

Recreates session from `.task.json` if session died, then `exec tmux attach`.

Success `data`:

```json
{"slug": "...", "session": "task-...", "was_recreated": false}
```

### `ds list`

Success `data`:

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

`session_state` ∈ `attached`, `detached`, `absent`.

### `ds finish <slug> [--force] [--keep-branches]`

Success `data`:

```json
{
  "slug": "...",
  "removed_worktrees": ["api", "web"],
  "killed_session": "task-...",
  "branches_kept": false
}
```

### `ds repos`

Success `data`:

```json
{
  "repos": [
    {"name": "api", "path": "/abs/path", "root": "flocasts"}
  ],
  "roots": ["/home/jordan/dev/flocasts"]
}
```

### `ds create-interactive`

`gum`-driven prompts compose existing commands; emits `new`'s envelope on completion.

## Skills layer

Lives in the repo under `skills/`. `npx skills add` copies into the user's Claude skills directory.

### Router: `skills/SKILL.md`

Positive triggers: tasks, tickets, worktrees, "spin up", "resume," "start work on," Jira IDs.

Negative triggers: factual questions ("what tasks do I have open") — run `ds list` directly, don't invoke a leaf skill.

### Leaf: `skills/task-new/SKILL.md`

Flow:

1. Fetch ticket via Atlassian MCP if referenced.
2. Infer conventional-commit type.
3. Propose slug from ticket summary.
4. Call `ds repos` to list available repos; propose likely ones using a hint table inside the SKILL.md.
5. Confirm slug, type, ticket, repos with the user.
6. Run `ds new <slug> --type <t> --ticket <id> --repos <...>`.
7. Parse envelope; report `attach_command`.

The skill parses JSON, never scrapes human output. If `{"ok": false}`, surface `error.code` and `error.message`, propose a fix where appropriate (e.g. `repo_not_found` → rerun with a different repo list).

#### Type inference table (initial)

- `Bug` issuetype → `fix`
- `Story` issuetype or label "feature" → `feat`
- `Task` issuetype, no other signal → `chore`
- `Epic` issuetype → `feat`
- `Improvement` issuetype or label "refactor" → `refactor`
- `Spike`, label "investigation" → `chore`
- label "docs" → `docs`
- label "test" → `test`
- fallback → `feat`

### Leaf: `skills/task-finish/SKILL.md`

Verifies merged state, confirms, runs `ds finish`. Parses envelope.

### Future leaves (not in v0.1)

- `task-handoff` — export/import across machines
- `task-review` — check out a PR as a task
- `task-orchard` — hand off to Orchard for agent dispatch

## Integration points

### Niri hotkey

Bind `Mod+T` to `ghostty -e ds pick`. Picker at TTY execs into tmux.

### SSH resume

`ssh endeavour -t ds pick`. `-t` forces a TTY so `fzf` renders.

### Claude Code

Natural language triggers the router skill, which dispatches to the right leaf.

## Open questions (non-blocking)

1. **`npx skills add` mechanics.** Does this tool exist already, or do we assume it does? If it doesn't, `install.sh` + a README one-liner is the fallback. The spec assumes the former; verify before release.
2. **Binary install path under `npx skills add`.** `~/.local/bin/ds` is the safest default. Open: does the installer check PATH and warn if missing?
3. **Disambiguation for duplicate repo names across roots.** `<root-basename>/<repo-name>` form accepted, or always require explicit namespacing? Current spec: only disambiguate when collision occurs.
4. **First-run experience.** When `repo_roots` is empty, the CLI emits `config_error`. Should it also offer to write a starter config to `~/.config/do-stuff/config.toml` via an interactive prompt? v0.1 answer: no — users either set it manually or via the skill.

## Milestones

### v0.0 (bootstrap) — see appendix

- Flake (flake-parts) with dev shell, **installable package output**, pre-commit hooks.
- `nix run github:jordangarrison/do-stuff` works against `main` from day one.
- `nix profile install github:jordangarrison/do-stuff` places the binary on PATH.
- GoReleaser config for cross-platform binaries.
- GitHub Actions: `ci.yml` and `release.yml`.
- Stub `cmd/ds/main.go` that prints the version.
- No Cobra, no real commands yet.

### v0.1

- Cobra wiring, `isatty` detection, envelope rendering.
- Commands: `new`, `list`, `pick`, `attach`, `repos`.
- All error codes wired with exit-code mapping.
- Router skill + `task-new` leaf under `skills/`.
- `install.sh` for the `npx skills add` install path.

### v0.2

- `finish` command + leaf.
- `create-interactive` command.
- Post-create hooks.
- home-manager module (`homeModules.default`).
- `schema_version` on envelopes.

### v0.3 (stretch)

- Session resurrection semantics when tmux died mid-task.
- `task-handoff` skill.
- `task-orchard` integration.
- Homebrew tap / Scoop bucket / signed releases.

## Success criteria for v0.1

1. From Claude: "start INFRA-6700, touches api and web." Ready-to-attach tmux session in under 10 seconds post-confirm.
2. From desktop hotkey: picker up, attach in under 2 seconds.
3. From SSH: `ds pick` resumes on a fresh shell.
4. Every CLI invocation returns a parseable envelope when piped.
5. Every error has a stable `code` agents can pattern-match on.
6. Resume paths never require Claude to be running.
7. Fresh install via `npx skills add github.com/jordangarrison/do-stuff` (or equivalent fallback) places both binary and skills correctly, first run succeeds after the user sets `repo_roots`.

---

# Appendix: Dev Environment and Release Tooling

This appendix enumerates the tooling the `do-stuff` repo needs before any CLI code gets written. It is a **dependency checklist** and a **decision record**, not implementation. The coding agent will read this, then write the actual `flake.nix`, CI workflows, and config files during the bootstrap phase.

Everything here is deliberately scoped to "what should exist and why," so the agent has enough context to produce the files without guessing at the shape of the project.

---

## Goals

- `nix develop` (or `direnv allow`) produces a working Go toolchain and every runtime dependency the CLI shells out to.
- Pre-commit hooks enforce formatting and lint rules so agent-generated code stays clean across iterations.
- The flake is structured to grow into a package output, a home-manager module, and CI-consumed derivations without restructuring.
- Contributors who don't use Nix can still build with `go build`, but the blessed path is Nix.
- Releases are fully automated: tag a version, get cross-platform binaries published to GitHub Releases, checksums, and (later) Homebrew / Nix / Scoop metadata.

## Non-goals

- Multi-arch matrix beyond `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64` at v0.1. Windows is deliberately out of scope.
- Signed releases in v0.1. Cosign / keyless signing lands when someone actually needs it.

## Flake structure

Uses `flake-parts`. Top-level inputs:

- `nixpkgs` — `nixos-unstable` channel (Go latest, no version pinning)
- `flake-parts` — `github:hercules-ci/flake-parts`
- `git-hooks` — `github:cachix/git-hooks.nix`
- `systems` — `github:nix-systems/default` (linux + darwin)

Flake outputs at v0.0 (bootstrap):

- `devShells.default` — the dev environment (see "Tools" below)
- `packages.default` — the `ds` binary, built via `pkgs.buildGoModule`
- `apps.default` — wired to `packages.default` so `nix run` works
- `checks.default` — runs pre-commit hooks and `go test`
- `checks.package` — builds `packages.default` (verifies packaging in CI)
- `formatter` — `nixpkgs-fmt` so `nix fmt` Just Works
- `overlays.default` — exposes `do-stuff` to downstream flakes

Flake outputs added in v0.2:

- `homeModules.default` — home-manager module that installs the binary and places `skills/` under the user's Claude skills directory

Flake-parts' `git-hooks.flakeModule` provides the `pre-commit` option tree. No bespoke hook wiring needed.

## Nix packaging

`packages.default` is a `buildGoModule` derivation. Specific requirements so it's actually usable, not just buildable:

- **`vendorHash`** pinned so builds are reproducible. The agent sets it via `lib.fakeHash` initially, runs `nix build`, and updates to the real hash on the failure message. Standard Nix dance.
- **`version`** read from a single source (e.g. a `VERSION` file or a `module.go` constant) so bumps don't require editing both Go code and Nix expressions. GoReleaser reads the same source.
- **`ldflags`** pass `-X main.version=${version}` so `ds --version` prints the right thing in Nix-built binaries the same way GoReleaser does.
- **`doCheck = true`** by default. `go test ./...` runs during `nix build`. Skippable via an override for fast iteration.
- **`meta`** populated: `description`, `homepage`, `license`, `mainProgram = "ds"`, `platforms`. Required for `nix profile install` to name the binary correctly and for downstream flakes that index by `meta`.
- **Runtime dependencies** listed in `buildInputs` where statically required. The CLI shells out to `git`, `tmux`, `fzf`, `jq`, `gum`, but we **do not** `wrapProgram` the binary with these. Users install them separately (or they come from the home-manager module in v0.2). Rationale: forcing a specific `git` or `tmux` via wrapping breaks users who intentionally use their own. The `meta.description` lists the runtime deps so the user knows what to install.

### Install paths that must work from day one

- `nix run github:jordangarrison/do-stuff -- --version`

- `nix run github:jordangarrison/do-stuff -- list` (will error cleanly since no config exists; the CLI itself must still load)

- `nix profile install github:jordangarrison/do-stuff`

- `nix shell github:jordangarrison/do-stuff -c ds --version`

- Downstream flake consumption:

    ```nix
    inputs.do-stuff.url = "github:jordangarrison/do-stuff";
    # then reference inputs.do-stuff.packages.${system}.default
    ```


### Install paths deferred to v0.2

- `home-manager` module that installs the binary **and** copies `skills/` into the user's Claude skills directory.
- NUR submission / nixpkgs PR (only if there's demand).

## Tools the dev shell provides

### Compile-time

- `go` — latest from unstable
- `gopls` — LSP for editor integration
- `gofumpt` — stricter gofmt
- `golangci-lint` — aggregate linter
- `gotools` — `goimports`, `stringer`, etc.
- `delve` — debugger

### Runtime (for local testing and manual invocation)

- `git` — worktree operations; >= 2.25 required
- `tmux` — session creation
- `fzf` — backs `ds pick`
- `jq` — useful for piping JSON output during manual testing
- `gum` — backs `ds create-interactive` (v0.2; present now for smoke tests)

### Nix housekeeping

- `nixpkgs-fmt` — flake formatter
- `deadnix` — dead-code linter for Nix files

### Release

- `goreleaser` — cross-compilation and release orchestration

## Pre-commit hooks

Managed by `git-hooks.nix`. Two phases:

### On commit (fast, enforced always)

- `gofumpt` on staged Go files
- `golangci-lint run ./...` (full project; fast with cache)
- `go vet ./...`
- Hygiene: trailing whitespace, end-of-file newline, large-file guard
- Nix hygiene: `nixpkgs-fmt`, `deadnix`

### On push (slower)

- `go test ./...`
- `go build ./...`

Hook installation happens automatically via the dev shell's `shellHook`. No manual `pre-commit install` step.

## golangci-lint configuration

Lives at `.golangci.yml`. Starting linter set:

- `govet`
- `staticcheck`
- `errcheck`
- `gofumpt`
- `unused`
- `ineffassign`
- `misspell`

Explicitly **disabled** from the start (these burn time without catching real bugs on a CLI of this shape):

- `gochecknoinits`
- `exhaustruct`
- `gochecknoglobals`
- `nlreturn`
- `wsl`

Adjust as the project grows. Document any deviation in a comment at the top of the file.

## Direnv

`.envrc` contains `use flake`. Users with `nix-direnv` get automatic shell activation on `cd`. The dev shell inherits editor tooling (`gopls`, formatters) so any editor with direnv integration picks up the right binaries without further config.

## .gitignore baseline

The bootstrap commit should cover at minimum:

```
.direnv/
.pre-commit-config.yaml   # generated by git-hooks.nix
result
result-*
/ds
/dist/                    # goreleaser output
*.test
coverage.out
coverage.html
```

## Release pipeline

### GoReleaser

`.goreleaser.yaml` at repo root. v0.1 configuration:

- **Builds:** `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`.
- **Archives:** `tar.gz` for Unix. Include `README.md`, `LICENSE`, and the `skills/` directory so the tarball is a complete install.
- **Checksums:** SHA256 manifest alongside each release.
- **Changelog:** auto-generated from commits, with conventional-commit groups (feat, fix, chore, refactor, etc.) matching the branch-type taxonomy the CLI itself uses. Nice symmetry — same vocabulary in branches, commit messages, and changelogs.
- **Snapshot mode:** enabled for local dry-runs without tagging.

Deferred to v0.2+:

- Homebrew tap publication
- Scoop bucket (if Windows ever matters)
- Nix flake auto-bump (via a separate workflow triggered by release)
- Cosign signing

### GitHub Actions

Three workflows under `.github/workflows/`:

**`ci.yml`** (every push, every PR):

- Check out repo
- Install Nix via `DeterminateSystems/nix-installer-action`
- Set up flake cache via `DeterminateSystems/flakehub-cache-action` or `cachix/cachix-action`
- Run `nix flake check` (which includes pre-commit + tests)
- Run `nix build .#default` to prove the package output builds

This workflow is Nix-native. CI uses the same derivations local devs use; no "works on my machine" drift between contributor and runner.

**`release.yml`** (on tag `v*`):

- Check out repo with full history (goreleaser needs it for changelogs)
- Install Nix (same pattern as CI)
- Enter the dev shell (`nix develop --command bash -c '...'`) so `goreleaser` runs with the pinned toolchain
- Run `goreleaser release --clean`
- GoReleaser handles GitHub Release creation, asset upload, and checksums using the built-in `GITHUB_TOKEN`

**`flake-update.yml`** (weekly cron, optional for v0.1):

- `nix flake update` and open a PR with the lock bump
- Reduces drift for long-term maintainability

### Versioning

- Semantic versioning. `v0.y.z` while pre-1.0.
- Git tags are the source of truth; GoReleaser reads the tag.
- `ds --version` prints the version stamped in at build time via `-ldflags "-X main.version=..."`. GoReleaser sets this automatically.

## Acceptance criteria for "bootstrap done"

Before any CLI implementation work begins, all of these must pass:

1. `nix flake check` — green (runs hooks + tests + package build).
2. `nix develop` or `direnv allow` activates the shell.
3. Inside the shell, `go version` prints a working toolchain.
4. `git`, `tmux`, `fzf`, `jq`, `gum`, `goreleaser` all on PATH in the shell.
5. `pre-commit run --all-files` succeeds (no Go files yet, but whitespace and Nix hooks still run).
6. A trivial `cmd/ds/main.go` prints `"ds v0.0.0"` and builds via **all three** paths:
    - `go build ./...` inside the dev shell
    - `nix build .#default` at the repo root
    - `nix run .# -- --version` prints the version
7. `go test ./...` passes (no tests yet, but no panic).
8. Committing triggers the fast hooks; pushing triggers the full test suite.
9. `nix run github:jordangarrison/do-stuff -- --version` works once the bootstrap commit lands on `main` and the repo is public. (The agent can verify the workflow locally with `nix run .# -- --version`; the remote path is trivially derivable once the repo exists.)
10. `nix profile install .#` succeeds and `ds --version` works outside the dev shell afterward. `nix profile remove do-stuff` cleans up.
11. `goreleaser release --snapshot --clean` produces a `dist/` directory with four binaries (linux/darwin × amd64/arm64) and a checksums file, without needing a tag or GitHub access.
12. Pushing a tag `v0.0.1-bootstrap` to the repo triggers `release.yml` and creates a draft release with all four platform archives attached. (The agent can verify workflow syntax passes `actionlint`.)

Only after all twelve pass does the agent move on to implementing commands per the main spec.

## File checklist for the bootstrap commit

First commit to the repo should contain exactly:

- `flake.nix` and `flake.lock`
- `.envrc`
- `.gitignore`
- `.golangci.yml`
- `.goreleaser.yaml`
- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`
- `go.mod` (from `go mod init github.com/jordangarrison/do-stuff`)
- `cmd/ds/main.go` — "hello world" stub that prints the version
- `README.md` — project description, install pointer, dev instructions
- `LICENSE` — MIT or Apache-2.0 (pick one; v0.1 proposal: MIT)

Nothing else. No cobra dependency, no config parsing, no subcommands. That's the first implementation commit, separate from bootstrap.

## Notes for the coding agent

- Do **not** add Go dependencies (cobra, toml libraries, testify, etc.) in the bootstrap commit. Those arrive with the first real implementation commit.
- Do **not** skip pre-commit setup because "there's no code yet." The rails need to exist before the first meaningful commit to work.
- If `flake-parts` or `git-hooks.nix` semantics have shifted by the time the agent reads this, consult the current docs at https://flake.parts/ and the `git-hooks.nix` README. The shape of the flake may need minor adjustments; the intent does not change.
- `goreleaser release --snapshot --clean` is the local smoke test. It requires zero credentials and produces real artifacts.
- The `release.yml` workflow should pin action SHAs or use Dependabot-managed floats, not bare `@main` references. Agent's judgment on which is appropriate.
- Do not skip the `.goreleaser.yaml` in bootstrap even though no release is planned for the commit itself. Having it means tagging v0.1 later is a one-line action.
