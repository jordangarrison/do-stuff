# AGENTS.md

Working notes for agent and human contributors. Short by design.

## Philosophy

- **Simple, not easy.** Rich Hickey, *Simple Made Easy*: simple = decomplected (one concern per thing). Not the same as "familiar" or "fewer keystrokes." Favor primitives that compose.
- **Complexity demon.** grugbrain.dev: three similar lines beats a bad abstraction; YAGNI; delete code before adding code; boring wins.
- **Unix compose.** `ds` is a thin Go wrapper over `git`, `tmux`, `fzf`. Don't reinvent what the underlying tools already do. If a shell pipeline would express it, the Go code probably should too.

## Repo shape

- **Three layers, each calls only the one beneath it.** Skills → CLI (`internal/cli`) → orchestration (`internal/task`) → primitives (`internal/git`, `internal/tmux`, `internal/errs`, `internal/config`, `internal/discover`).
- **Orchestrators compose primitives.** They don't call each other or reach sideways. `task.Create`, `task.Attach`, `task.Finish` each own one verb.
- **Envelope is the contract.** `ok` / `command` / `data` / `error` with stable `code` strings. Messages and `details` are free-form. Adding fields is additive; breaking existing ones needs `schema_version`.
- **One file, one job.** Split when a file grows multiple responsibilities. Focused files are easier for everyone (agents included) to reason about.
- **Mark the mutation boundary.** When a function crosses from preflight checks into filesystem / tmux / git mutation, leave a `// State mutation begins here.` comment. See `task.Create` for the pattern.

## Anti-patterns

- Premature abstraction (wait for three uses).
- Hidden state (pass it through params or opts structs).
- Backwards-compat shims (we're pre-1.0; change the code).
- Error handling for scenarios that can't happen.
- Comments that narrate *what* the code does; only write comments for *why*.
- Multi-paragraph docstrings. One short line, rare.

## Testing

- Hermetic by default. `t.TempDir()`, `testutil.InitFixtureRepo`, table-driven where natural.
- Gate tmux/git-requiring tests with presence checks. See `requireTmux(t)` in `internal/task/attach_test.go`.
- Inject `execFn` / `lookupFn` / `selectorFn` / `AttachFn` / `FinishFn`. Never call `syscall.Exec` or spawn real `fzf` in tests.
- `go test ./...` must be green at every commit.

## Commits

- Conventional Commits with repo scopes: `tmux`, `git`, `task`, `cli`, `errs`, `config`, `discover`, `docs`, `refactor(<scope>)`.
- Ticket IDs in the message when relevant.
- No Claude co-author. No `--no-verify`, no `--force` push.
- Fix broken pre-commit hooks rather than skipping them.

## Agent workflow

- Skills live in `skills/` (router + leaves). They shell out to `ds` and parse envelopes.
- Design flow: `superpowers:brainstorming` → `superpowers:writing-plans` → `superpowers:subagent-driven-development` or `superpowers:executing-plans`.
- Specs: `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md`.
- Plans: `docs/superpowers/plans/YYYY-MM-DD-<topic>.md`.

## Pointers

- `SPEC.md` — canonical product/behavior spec.
- `docs/superpowers/specs/` — per-slice designs.
- `docs/superpowers/plans/` — per-slice implementation plans.
