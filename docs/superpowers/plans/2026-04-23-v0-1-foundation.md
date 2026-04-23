# do-stuff v0.1 Slice 1 — Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lay the v0.1 CLI foundation — Cobra wiring, envelope rendering, isatty detection, full error-code/exit-code table, config loading (YAML), repo discovery, and the first real command `ds repos`. Also absorb the bootstrap carry-over and known tech debt so subsequent slices ride on clean rails.

**Architecture:** Thin CLI (`internal/cli`) calls into deterministic core packages (`internal/config`, `internal/discover`, `internal/errs`). Every command emits a structured envelope via `internal/cli/output.go` and returns exit codes via `errs.TaskError.ExitCode()`. Cobra `RunE` returns error; `main` calls `Render` then `os.Exit`. No `internal/task`, `internal/git`, or `internal/tmux` in this slice — those arrive with slice 2 (`new`/`list`/`pick`/`attach`).

**Tech Stack:** Go (Go 1.26 per `go.mod`), Cobra v1, `gopkg.in/yaml.v3`, `golang.org/x/term` for isatty. YAML config (spec change: was TOML). Nix `buildGoModule` for packaging; overlay refactored to share a single derivation definition.

**Reference spec:** `/home/jordangarrison/dev/jordangarrison/brain/00-inbox/do-stuff - Task Tooling Spec (v0.5).md` — "Milestones > v0.1" + "Output contract" + "Data model > Configuration" + "Go project layout". **Spec deviations documented in Task 6 (YAML instead of TOML) and Task 1 (version resolver).**

**Out of scope (later slices):** `ds new`, `ds list`, `ds pick`, `ds attach`, `ds finish`, `skills/`, `install.sh`. See `docs/superpowers/plans/2026-04-23-v0-1-commands.md` (slice 2, TBW) and `2026-04-23-v0-1-skills.md` (slice 3, TBW).

---

## Final file tree at end of slice 1

```
do-stuff/
  cmd/ds/
    main.go                # cobra root wiring + exec
    version.go             # resolveVersion()
    version_test.go
  internal/
    cli/
      root.go              # cobra root Command + global flags
      output.go            # Envelope, Render, TTY detect
      output_test.go
      repos.go             # ds repos command
      repos_test.go
    config/
      config.go            # YAML load, XDG, defaults, path expand
      config_test.go
      testdata/
        valid.yaml
        invalid.yaml
    discover/
      walk.go               # depth-2 .git walk + disambiguation
      walk_test.go
    errs/
      errs.go               # Code + TaskError + ExitCode
      errs_test.go
  go.mod, go.sum             # cobra, yaml.v3, x/term pinned
  flake.nix                  # shared buildGoModule let-binding + real vendorHash
  docs/superpowers/plans/
    2026-04-23-v0-1-foundation.md   # this file
  ...                        # everything from bootstrap unchanged
```

---

## Task 1: Version resolver with BuildInfo fallback (bootstrap carry-over)

Rationale: the bootstrap stub hardcoded `var version = "v0.0.0"`, so `go install github.com/jordangarrison/do-stuff/cmd/ds@vX.Y.Z` reports a wrong version when ldflags aren't applied. Fix: sentinel `"dev"` default + `runtime/debug.ReadBuildInfo` fallback. Nix and GoReleaser keep their explicit `-X main.version=...` ldflag overrides (those already work and win over the sentinel).

**Files:**
- Create: `cmd/ds/version.go`
- Create: `cmd/ds/version_test.go`
- Modify: `cmd/ds/main.go`

- [ ] **Step 1: Write failing test `cmd/ds/version_test.go`**

File: `cmd/ds/version_test.go`
```go
package main

import "testing"

func TestResolveVersion_ldflagWins(t *testing.T) {
	got := resolveVersionFrom("v1.2.3", func() (string, bool) { return "v9.9.9", true })
	if got != "v1.2.3" {
		t.Fatalf("want v1.2.3, got %s", got)
	}
}

func TestResolveVersion_sentinelFallsBackToBuildInfo(t *testing.T) {
	got := resolveVersionFrom("dev", func() (string, bool) { return "v2.0.0", true })
	if got != "v2.0.0" {
		t.Fatalf("want v2.0.0, got %s", got)
	}
}

func TestResolveVersion_develBuildInfoIgnored(t *testing.T) {
	got := resolveVersionFrom("dev", func() (string, bool) { return "(devel)", true })
	if got != "dev" {
		t.Fatalf("want dev, got %s", got)
	}
}

func TestResolveVersion_emptyBuildInfoStaysDev(t *testing.T) {
	got := resolveVersionFrom("dev", func() (string, bool) { return "", false })
	if got != "dev" {
		t.Fatalf("want dev, got %s", got)
	}
}
```

- [ ] **Step 2: Run test, expect FAIL (undefined function)**

Run: `go test ./cmd/ds -run TestResolveVersion -v`
Expected: compile error `undefined: resolveVersionFrom`.

- [ ] **Step 3: Implement `cmd/ds/version.go`**

File: `cmd/ds/version.go`
```go
package main

import "runtime/debug"

// version is overridden at build time via -ldflags "-X main.version=..."
// by Nix and GoReleaser. `go install` / `go build` leave it as the sentinel,
// at which point resolveVersion falls back to runtime/debug.ReadBuildInfo.
var version = "dev"

func resolveVersion() string {
	return resolveVersionFrom(version, func() (string, bool) {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return "", false
		}
		return bi.Main.Version, true
	})
}

func resolveVersionFrom(ldflag string, buildInfo func() (string, bool)) string {
	if ldflag != "dev" {
		return ldflag
	}
	v, ok := buildInfo()
	if !ok {
		return "dev"
	}
	if v == "" || v == "(devel)" {
		return "dev"
	}
	return v
}
```

- [ ] **Step 4: Run test, expect PASS**

Run: `go test ./cmd/ds -run TestResolveVersion -v`
Expected: `PASS` on all four subtests.

- [ ] **Step 5: Update `cmd/ds/main.go` to use `resolveVersion()`**

File: `cmd/ds/main.go`
```go
package main

import "fmt"

func main() {
	fmt.Printf("ds %s\n", resolveVersion())
}
```

Delete the old `var version = "v0.0.0"` line (now lives in `version.go` as `"dev"`).

- [ ] **Step 6: Verify all three build paths still print a version string**

Run: `go build -o /tmp/ds-go ./cmd/ds && /tmp/ds-go && rm /tmp/ds-go`
Expected: `ds dev` (BuildInfo.Main.Version is `(devel)` for local `go build`, so sentinel stays).

Run: `nix build .#default && ./result/bin/ds`
Expected: `ds v0.0.0` (nix ldflag still overrides; will update to real version when v0.1 tag ships).

Run: `nix run .# -- --version`
Expected: `ds v0.0.0`.

- [ ] **Step 7: Commit**

```bash
git add cmd/ds/main.go cmd/ds/version.go cmd/ds/version_test.go
git commit -m "fix: resolve ds version from buildinfo when ldflags absent"
```

---

## Task 2: Flake overlay refactor — share buildGoModule derivation

Rationale: `flake.nix` currently duplicates the `buildGoModule` block — once in `packages.default` and again in `overlays.default`. Bumping the version or tweaking meta means two edits with an easy-to-miss drift. Hoist the shared args into a function-of-pkgs `let`-binding so both outputs call it.

**Files:**
- Modify: `flake.nix`

- [ ] **Step 1: Read current `flake.nix` and identify duplication**

Run: `cat flake.nix | head -200`
Expected: two separate `buildGoModule { pname = "do-stuff"; ... }` expressions (one in `perSystem.packages.default`, one in `flake.overlays.default`).

- [ ] **Step 2: Rewrite `flake.nix` with shared `mkDs` factory at top-level `let`**

The factory lives outside `perSystem` so both `packages.default` (via `mkDs pkgs`) and `overlays.default` (via `mkDs prev`) can call it. `pkgs` is threaded at call site.

File: `flake.nix` (full replacement)

```nix
{
  description = "do-stuff: task-based multi-repo worktree manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts.url = "github:hercules-ci/flake-parts";
    git-hooks.url = "github:cachix/git-hooks.nix";
    systems.url = "github:nix-systems/default";
  };

  outputs =
    inputs@{ flake-parts, systems, ... }:
    let
      # Single source of truth for the ds build. Callers pass pkgs so both
      # perSystem outputs and downstream overlay consumers can reuse this.
      mkDs = pkgs: pkgs.buildGoModule {
        pname = "do-stuff";
        version = "0.0.0";
        src = ./.;
        vendorHash = null; # updated in Task 4 when first dep lands
        subPackages = [ "cmd/ds" ];
        ldflags = [
          "-s"
          "-w"
          "-X main.version=v0.0.0"
        ];
        doCheck = true;
        meta = with pkgs.lib; {
          description = "Task-based multi-repo worktree manager";
          homepage = "https://github.com/jordangarrison/do-stuff";
          license = licenses.mit;
          mainProgram = "ds";
          platforms = platforms.unix;
        };
      };
    in
    flake-parts.lib.mkFlake { inherit inputs; } {
      systems = import systems;

      imports = [ inputs.git-hooks.flakeModule ];

      perSystem =
        { config, pkgs, self', ... }:
        {
          pre-commit = {
            check.enable = true;
            settings.hooks = {
              gofumpt = {
                enable = true;
                name = "gofumpt";
                entry = "${pkgs.gofumpt}/bin/gofumpt -l -w";
                language = "system";
                types = [ "go" ];
              };
              golangci-lint = {
                enable = true;
                pass_filenames = false;
                entry = pkgs.lib.mkForce (
                  let
                    script = pkgs.writeShellScript "precommit-golangci-lint" ''
                      export PATH="${pkgs.go}/bin:$PATH"
                      exec ${pkgs.golangci-lint}/bin/golangci-lint run ./...
                    '';
                  in
                  builtins.toString script
                );
              };
              govet = {
                enable = true;
                name = "go vet";
                entry = "${pkgs.go}/bin/go vet ./...";
                language = "system";
                pass_filenames = false;
                types = [ "go" ];
              };
              trim-trailing-whitespace.enable = true;
              end-of-file-fixer.enable = true;
              nixpkgs-fmt.enable = true;
              deadnix.enable = true;

              go-test = {
                enable = true;
                name = "go test";
                entry =
                  let
                    script = pkgs.writeShellScript "prepush-go-test" ''
                      [ -f go.mod ] || { echo "no go.mod, skipping go test"; exit 0; }
                      exec ${pkgs.go}/bin/go test ./...
                    '';
                  in
                  builtins.toString script;
                language = "system";
                pass_filenames = false;
                stages = [ "pre-push" ];
              };
              go-build = {
                enable = true;
                name = "go build";
                entry =
                  let
                    script = pkgs.writeShellScript "prepush-go-build" ''
                      [ -f go.mod ] || { echo "no go.mod, skipping go build"; exit 0; }
                      exec ${pkgs.go}/bin/go build ./...
                    '';
                  in
                  builtins.toString script;
                language = "system";
                pass_filenames = false;
                stages = [ "pre-push" ];
              };
            };
          };

          packages.default = mkDs pkgs;

          apps.default = {
            type = "app";
            program = "${self'.packages.default}/bin/ds";
          };

          checks.package = self'.packages.default;

          devShells.default = pkgs.mkShell {
            inputsFrom = [ self'.packages.default ];
            packages = with pkgs; [
              gopls
              gofumpt
              golangci-lint
              gotools
              delve

              git
              tmux
              fzf
              jq
              gum

              nixpkgs-fmt
              deadnix
              pre-commit

              goreleaser
            ];

            shellHook = config.pre-commit.installationScript;
          };

          formatter = pkgs.nixpkgs-fmt;
        };

      flake = {
        overlays.default = _final: prev: {
          do-stuff = mkDs prev;
        };
      };
    };
}
```

Single `mkDs pkgs` factory used by both `packages.default` and `overlays.default`. Top-level `let` is evaluated once per flake eval; `pkgs` is passed in at call site.

- [ ] **Step 3: Verify `nix build .#default` still succeeds**

Run: `nix build .#default`
Expected: builds `result/bin/ds`, no eval errors, same binary contents.

Run: `./result/bin/ds`
Expected: `ds v0.0.0`.

- [ ] **Step 4: Verify overlay still works (smoke test via `nix eval`)**

Run: `nix eval .#overlays.default --apply 'f: builtins.isFunction f'`
Expected: `true`.

- [ ] **Step 5: Verify `nix flake check` green**

Run: `nix flake check --print-build-logs`
Expected: all checks pass.

- [ ] **Step 6: Commit**

```bash
git add flake.nix
git commit -m "refactor: hoist ds derivation into shared mkDs factory"
```

---

## Task 3: Error package `internal/errs`

Defines `Code` type, all thirteen v0.1 codes, `TaskError` struct, `ExitCode()` method mapping. No deps beyond stdlib. Every subsequent package returns `*TaskError` for typed failures; unwrapped `error`s become `internal_error` / exit 1 at the CLI boundary.

**Files:**
- Create: `internal/errs/errs.go`
- Create: `internal/errs/errs_test.go`

- [ ] **Step 1: Write failing test `internal/errs/errs_test.go`**

File: `internal/errs/errs_test.go`
```go
package errs

import (
	"errors"
	"testing"
)

func TestExitCode_table(t *testing.T) {
	cases := []struct {
		code Code
		want int
	}{
		{InvalidArgs, 2},
		{RepoNotFound, 3},
		{TaskExists, 4},
		{BranchConflict, 5},
		{WorktreeExists, 5},
		{TmuxUnavailable, 6},
		{TmuxSessionExists, 6},
		{TmuxSessionMissing, 6},
		{GitError, 7},
		{WorktreeDirty, 7},
		{ConfigError, 8},
		{TaskNotFound, 9},
		{Internal, 1},
	}
	for _, c := range cases {
		t.Run(string(c.code), func(t *testing.T) {
			got := (&TaskError{Code: c.code}).ExitCode()
			if got != c.want {
				t.Fatalf("code %s: want %d, got %d", c.code, c.want, got)
			}
		})
	}
}

func TestExitCode_unknownCodeIsOne(t *testing.T) {
	got := (&TaskError{Code: "bogus_not_a_real_code"}).ExitCode()
	if got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestError_implementsError(t *testing.T) {
	var err error = &TaskError{Code: InvalidArgs, Message: "bad flag"}
	if err.Error() != "bad flag" {
		t.Fatalf("want 'bad flag', got %q", err.Error())
	}
}

func TestError_errorsAsWorks(t *testing.T) {
	var err error = &TaskError{Code: ConfigError, Message: "no config"}
	var te *TaskError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As failed to unwrap TaskError")
	}
	if te.Code != ConfigError {
		t.Fatalf("want ConfigError, got %s", te.Code)
	}
}
```

- [ ] **Step 2: Run test, expect compile failure**

Run: `go test ./internal/errs/...`
Expected: `undefined: Code`, `undefined: TaskError`, etc.

- [ ] **Step 3: Implement `internal/errs/errs.go`**

File: `internal/errs/errs.go`
```go
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
```

- [ ] **Step 4: Run test, expect PASS**

Run: `go test ./internal/errs/... -v`
Expected: all subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/errs/errs.go internal/errs/errs_test.go
git commit -m "feat(errs): add TaskError + exit-code mapping per spec"
```

---

## Task 4: Envelope + isatty rendering (`internal/cli/output.go`)

First task that adds a Go dep: `golang.org/x/term` for `isatty`. We update `go.mod`, refresh Nix `vendorHash`, and ship the envelope types + `Render` function that every subsequent command calls.

**Files:**
- Create: `internal/cli/output.go`
- Create: `internal/cli/output_test.go`
- Modify: `go.mod`, `go.sum`
- Modify: `flake.nix` (real `vendorHash`)

- [ ] **Step 1: Add dep and tidy**

Run: `nix develop --command bash -c 'go get golang.org/x/term && go mod tidy'`
Expected: `go.mod` gains a `require golang.org/x/term vX.Y.Z` line; `go.sum` gets entries.

- [ ] **Step 2: Write failing test `internal/cli/output_test.go`**

File: `internal/cli/output_test.go`
```go
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

func TestRender_successJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	data := map[string]string{"slug": "foo"}

	code := Render(RenderOpts{
		Command: "ds.repos",
		Data:    data,
		Err:     nil,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Mode:    ModeJSON,
	})

	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if !env.OK || env.Command != "ds.repos" {
		t.Fatalf("bad envelope: %+v", env)
	}
}

func TestRender_taskErrorMapsToExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	te := &errs.TaskError{Code: errs.ConfigError, Message: "no config"}

	code := Render(RenderOpts{
		Command: "ds.repos",
		Err:     te,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Mode:    ModeJSON,
	})

	if code != 8 {
		t.Fatalf("want exit 8, got %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if env.OK || env.Error == nil || env.Error.Code != errs.ConfigError {
		t.Fatalf("bad envelope: %+v", env)
	}
}

func TestRender_plainErrorBecomesInternal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Render(RenderOpts{
		Command: "ds.repos",
		Err:     errors.New("oops"),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Mode:    ModeJSON,
	})

	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if env.Error.Code != errs.Internal {
		t.Fatalf("want internal_error, got %s", env.Error.Code)
	}
}

func TestRender_humanModeSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Render(RenderOpts{
		Command: "ds.repos",
		Data:    map[string]string{"slug": "foo"},
		Stdout:  &stdout,
		Stderr:  &stderr,
		Mode:    ModeHuman,
	})

	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}
	if len(stdout.Bytes()) == 0 {
		t.Fatalf("human mode wrote nothing to stdout")
	}
	// Human success should not be valid JSON envelope (it's formatted text).
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err == nil && env.OK {
		t.Fatalf("human stdout unexpectedly parsed as success envelope")
	}
}

func TestRender_humanModeError_writesJSONToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	te := &errs.TaskError{Code: errs.RepoNotFound, Message: "missing"}

	code := Render(RenderOpts{
		Command: "ds.repos",
		Err:     te,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Mode:    ModeHuman,
	})

	if code != 3 {
		t.Fatalf("want exit 3, got %d", code)
	}
	// Human-mode errors go to stderr (structured for machine eyes) and stdout gets a short line.
	if stderr.Len() == 0 {
		t.Fatalf("stderr empty on error")
	}
}

func TestDetectMode_jsonFlagWins(t *testing.T) {
	m := DetectMode(DetectOpts{IsTerminal: true, JSON: true, Human: false})
	if m != ModeJSON {
		t.Fatalf("want ModeJSON, got %s", m)
	}
}

func TestDetectMode_humanFlagWins(t *testing.T) {
	m := DetectMode(DetectOpts{IsTerminal: false, JSON: false, Human: true})
	if m != ModeHuman {
		t.Fatalf("want ModeHuman, got %s", m)
	}
}

func TestDetectMode_ttyIsHuman(t *testing.T) {
	m := DetectMode(DetectOpts{IsTerminal: true})
	if m != ModeHuman {
		t.Fatalf("want ModeHuman, got %s", m)
	}
}

func TestDetectMode_pipeIsJSON(t *testing.T) {
	m := DetectMode(DetectOpts{IsTerminal: false})
	if m != ModeJSON {
		t.Fatalf("want ModeJSON, got %s", m)
	}
}
```

- [ ] **Step 3: Run test, expect compile failure**

Run: `go test ./internal/cli/...`
Expected: `undefined: Render`, `undefined: Envelope`, etc.

- [ ] **Step 4: Implement `internal/cli/output.go`**

File: `internal/cli/output.go`
```go
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

// Envelope is the structured response emitted by every ds command.
type Envelope struct {
	OK      bool             `json:"ok"`
	Command string           `json:"command"`
	Data    any              `json:"data,omitempty"`
	Error   *errs.TaskError  `json:"error,omitempty"`
}

// Mode controls whether Render emits pretty text or the JSON envelope.
type Mode string

const (
	ModeHuman Mode = "human"
	ModeJSON  Mode = "json"
)

// DetectOpts feeds DetectMode. Tests pass IsTerminal directly; production
// callers derive it from term.IsTerminal(int(os.Stdout.Fd())).
type DetectOpts struct {
	IsTerminal bool
	JSON       bool
	Human      bool
}

// DetectMode resolves the effective output mode: explicit flags win, otherwise
// a TTY on stdout means human and a pipe/redirect means JSON.
func DetectMode(o DetectOpts) Mode {
	switch {
	case o.JSON:
		return ModeJSON
	case o.Human:
		return ModeHuman
	case o.IsTerminal:
		return ModeHuman
	default:
		return ModeJSON
	}
}

// IsStdoutTerminal reports whether the current process's stdout is a TTY.
// Production entry point for DetectOpts.IsTerminal.
func IsStdoutTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// RenderOpts bundles everything Render needs. Stdout/Stderr are parameterized
// so tests can assert on captured buffers without touching global state.
type RenderOpts struct {
	Command string
	Data    any
	Err     error
	Stdout  io.Writer
	Stderr  io.Writer
	Mode    Mode
}

// Render writes the envelope (or human rendering) and returns the exit code.
// Callers are expected to `os.Exit(code)` on the return value.
func Render(o RenderOpts) int {
	env := Envelope{Command: o.Command}
	exitCode := 0

	if o.Err != nil {
		var te *errs.TaskError
		if !errors.As(o.Err, &te) {
			te = &errs.TaskError{
				Code:    errs.Internal,
				Message: o.Err.Error(),
			}
		}
		env.OK = false
		env.Error = te
		exitCode = te.ExitCode()
	} else {
		env.OK = true
		env.Data = o.Data
	}

	switch o.Mode {
	case ModeJSON:
		writeJSON(o.Stdout, env)
		if env.Error != nil {
			// one-line human summary to stderr for humans who grep logs
			fmt.Fprintf(o.Stderr, "error: %s (%s)\n", env.Error.Message, env.Error.Code)
		}
	case ModeHuman:
		if env.Error != nil {
			fmt.Fprintf(o.Stdout, "error: %s\n", env.Error.Message)
			writeJSON(o.Stderr, env)
		} else {
			writeHumanSuccess(o.Stdout, env)
		}
	}

	return exitCode
}

func writeJSON(w io.Writer, env Envelope) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(env)
}

// writeHumanSuccess emits a terse human-friendly summary. Each command can
// swap this for a richer rendering by passing Mode=ModeJSON and handling
// human output itself — for v0.1 slice 1 the default is enough.
func writeHumanSuccess(w io.Writer, env Envelope) {
	fmt.Fprintf(w, "ok: %s\n", env.Command)
	if env.Data != nil {
		b, err := json.MarshalIndent(env.Data, "", "  ")
		if err != nil {
			fmt.Fprintf(w, "  (data unmarshalable: %v)\n", err)
			return
		}
		fmt.Fprintln(w, string(b))
	}
}
```

- [ ] **Step 5: Run test, expect PASS**

Run: `go test ./internal/cli/... -v`
Expected: all subtests PASS.

- [ ] **Step 6: Refresh Nix `vendorHash`**

Edit `flake.nix`: change `vendorHash = null;` to `vendorHash = pkgs.lib.fakeHash;`.

Run: `nix build .#default`
Expected: build fails with `hash mismatch in fixed-output derivation` showing `got: sha256-XXXXX=`.

Copy the `got:` hash. Edit `flake.nix`: replace `pkgs.lib.fakeHash` with the real hash string.

Run: `nix build .#default`
Expected: succeeds. `./result/bin/ds` prints `ds v0.0.0`.

- [ ] **Step 7: Verify `nix flake check` green**

Run: `nix flake check --print-build-logs`
Expected: all checks pass (including `checks.package` which rebuilds with the real `vendorHash`).

- [ ] **Step 8: Commit**

```bash
git add internal/cli/output.go internal/cli/output_test.go go.mod go.sum flake.nix
git commit -m "feat(cli): add envelope renderer with tty detection"
```

---

## Task 5: Config package `internal/config`

YAML loader with XDG support, defaults, `~` / `$HOME` expansion in path fields. Adds `gopkg.in/yaml.v3` dep and refreshes `vendorHash`.

Deviation from spec: spec uses TOML; we use YAML for better hand-editing ergonomics (comments + no TOML syntax recall friction). `.task.json` stays JSON.

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/config/testdata/valid.yaml`
- Create: `internal/config/testdata/invalid.yaml`
- Modify: `go.mod`, `go.sum`, `flake.nix` (vendorHash)

- [ ] **Step 1: Add YAML dep**

Run: `nix develop --command bash -c 'go get gopkg.in/yaml.v3 && go mod tidy'`
Expected: `go.mod` gains yaml.v3.

- [ ] **Step 2: Create testdata fixtures**

File: `internal/config/testdata/valid.yaml`
```yaml
# do-stuff config (hand-edited)
tasks_dir: ~/.do-stuff
repo_roots:
  - ~/dev/flocasts
  - $HOME/dev/personal
tmux_prefix: task-
default_base: main
default_type: feat
start_tmux: true
```

File: `internal/config/testdata/invalid.yaml`
```yaml
tasks_dir: ~/.do-stuff
repo_roots:
  - ~/dev
  this is not valid yaml: [unclosed
```

- [ ] **Step 3: Write failing test `internal/config/config_test.go`**

File: `internal/config/config_test.go`
```go
package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

func TestLoad_missingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load("/nonexistent/path/does/not/exist.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TasksDir == "" {
		t.Fatal("TasksDir should have a default")
	}
	if cfg.TmuxPrefix != "task-" {
		t.Fatalf("want task-, got %q", cfg.TmuxPrefix)
	}
	if cfg.DefaultBase != "main" {
		t.Fatalf("want main, got %q", cfg.DefaultBase)
	}
	if cfg.DefaultType != "feat" {
		t.Fatalf("want feat, got %q", cfg.DefaultType)
	}
	if !cfg.StartTmux {
		t.Fatal("StartTmux should default true")
	}
	if len(cfg.RepoRoots) != 0 {
		t.Fatalf("RepoRoots should default empty, got %v", cfg.RepoRoots)
	}
}

func TestLoad_validFileParses(t *testing.T) {
	cfg, err := Load("testdata/valid.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.RepoRoots) != 2 {
		t.Fatalf("want 2 roots, got %d", len(cfg.RepoRoots))
	}
	home, _ := os.UserHomeDir()
	if cfg.TasksDir != filepath.Join(home, ".do-stuff") {
		t.Fatalf("want tilde expanded, got %q", cfg.TasksDir)
	}
	if cfg.RepoRoots[1] != filepath.Join(home, "dev/personal") {
		t.Fatalf("want $HOME expanded, got %q", cfg.RepoRoots[1])
	}
}

func TestLoad_invalidYAMLReturnsConfigError(t *testing.T) {
	_, err := Load("testdata/invalid.yaml")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) {
		t.Fatalf("want *errs.TaskError, got %T", err)
	}
	if te.Code != errs.ConfigError {
		t.Fatalf("want ConfigError, got %s", te.Code)
	}
}

func TestExpandPath_tilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandPath("~/foo")
	if got != filepath.Join(home, "foo") {
		t.Fatalf("want %s/foo, got %s", home, got)
	}
}

func TestExpandPath_home(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandPath("$HOME/foo")
	if got != filepath.Join(home, "foo") {
		t.Fatalf("want %s/foo, got %s", home, got)
	}
}

func TestExpandPath_absoluteUnchanged(t *testing.T) {
	got := expandPath("/abs/path")
	if got != "/abs/path" {
		t.Fatalf("want /abs/path, got %s", got)
	}
}

func TestDefaultPath_honorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	p := DefaultPath()
	if p != "/tmp/xdg-test/do-stuff/config.yaml" {
		t.Fatalf("want /tmp/xdg-test/do-stuff/config.yaml, got %s", p)
	}
}

func TestDefaultPath_fallsBackToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	p := DefaultPath()
	want := filepath.Join(home, ".config/do-stuff/config.yaml")
	if p != want {
		t.Fatalf("want %s, got %s", want, p)
	}
}
```

- [ ] **Step 4: Run test, expect compile failure**

Run: `go test ./internal/config/...`
Expected: `undefined: Load`, `undefined: expandPath`, `undefined: DefaultPath`.

- [ ] **Step 5: Implement `internal/config/config.go`**

File: `internal/config/config.go`
```go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

// Config is the materialized config consumed by the CLI. All fields are
// defaulted during Load — callers never need to check for zero values.
type Config struct {
	TasksDir    string
	RepoRoots   []string
	TmuxPrefix  string
	DefaultBase string
	DefaultType string
	StartTmux   bool
}

// rawConfig mirrors the on-disk YAML. Pointers let us detect absent keys
// versus zero values (critical for bools: `start_tmux: false` must override
// the default `true`).
type rawConfig struct {
	TasksDir    *string  `yaml:"tasks_dir"`
	RepoRoots   []string `yaml:"repo_roots"`
	TmuxPrefix  *string  `yaml:"tmux_prefix"`
	DefaultBase *string  `yaml:"default_base"`
	DefaultType *string  `yaml:"default_type"`
	StartTmux   *bool    `yaml:"start_tmux"`
}

// DefaultPath resolves the config location. Honors $XDG_CONFIG_HOME, falls
// back to ~/.config/do-stuff/config.yaml.
func DefaultPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "do-stuff", "config.yaml")
}

// Load reads the YAML config at path. Missing file returns defaults with no
// error; parse failures return a *TaskError{Code: ConfigError}.
func Load(path string) (*Config, error) {
	cfg := defaults()

	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, &errs.TaskError{
			Code:    errs.ConfigError,
			Message: fmt.Sprintf("reading %s: %v", path, err),
			Details: map[string]any{"path": path},
		}
	}

	var raw rawConfig
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, &errs.TaskError{
			Code:    errs.ConfigError,
			Message: fmt.Sprintf("parsing %s: %v", path, err),
			Details: map[string]any{"path": path},
		}
	}

	apply(cfg, &raw)
	expandPaths(cfg)
	return cfg, nil
}

func defaults() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		TasksDir:    filepath.Join(home, ".do-stuff"),
		RepoRoots:   []string{},
		TmuxPrefix:  "task-",
		DefaultBase: "main",
		DefaultType: "feat",
		StartTmux:   true,
	}
}

// apply copies any explicitly-set fields from raw onto dst. Pointers are
// nil when the key is absent from YAML, so an explicit `start_tmux: false`
// overrides the default `true` while omitting the key leaves the default.
func apply(dst *Config, raw *rawConfig) {
	if raw.TasksDir != nil {
		dst.TasksDir = *raw.TasksDir
	}
	if raw.RepoRoots != nil {
		dst.RepoRoots = raw.RepoRoots
	}
	if raw.TmuxPrefix != nil {
		dst.TmuxPrefix = *raw.TmuxPrefix
	}
	if raw.DefaultBase != nil {
		dst.DefaultBase = *raw.DefaultBase
	}
	if raw.DefaultType != nil {
		dst.DefaultType = *raw.DefaultType
	}
	if raw.StartTmux != nil {
		dst.StartTmux = *raw.StartTmux
	}
}

func expandPaths(c *Config) {
	c.TasksDir = expandPath(c.TasksDir)
	for i, r := range c.RepoRoots {
		c.RepoRoots[i] = expandPath(r)
	}
}

func expandPath(p string) string {
	if p == "" {
		return p
	}
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	if p == "~" {
		return home
	}
	return os.ExpandEnv(p)
}
```

- [ ] **Step 6: Run tests, expect PASS**

Run: `nix develop --command bash -c 'go test ./internal/config/... -v'`
Expected: all subtests PASS.

- [ ] **Step 7: Refresh Nix `vendorHash`**

Edit `flake.nix`: change `vendorHash = "sha256-...";` back to `vendorHash = pkgs.lib.fakeHash;`.

Run: `nix build .#default`
Expected: hash-mismatch failure printing new `got: sha256-...`.

Paste new hash into `flake.nix`; run `nix build .#default` again.
Expected: builds.

- [ ] **Step 8: `nix flake check` green**

Run: `nix flake check --print-build-logs`
Expected: pass.

- [ ] **Step 9: Commit**

```bash
git add internal/config go.mod go.sum flake.nix
git commit -m "feat(config): add yaml loader with xdg + tilde expansion"
```

---

## Task 6: Discover package `internal/discover`

Depth-2 walk of configured roots finding `.git` directories. Disambiguates duplicate repo names across roots as `<root-basename>/<repo-name>`. No new deps (stdlib only).

**Files:**
- Create: `internal/discover/walk.go`
- Create: `internal/discover/walk_test.go`

- [ ] **Step 1: Write failing test `internal/discover/walk_test.go`**

File: `internal/discover/walk_test.go`
```go
package discover

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// makeRepo creates a fake repo (dir with .git subdir) at dir/name.
func makeRepo(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Join(p, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWalk_emptyRoots(t *testing.T) {
	repos, err := Walk(nil)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(repos) != 0 {
		t.Fatalf("want empty, got %v", repos)
	}
}

func TestWalk_singleRootSingleRepo(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	repos, err := Walk([]string{root})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "api" {
		t.Fatalf("want [api], got %+v", repos)
	}
	if repos[0].Path != filepath.Join(root, "api") {
		t.Fatalf("wrong path: %s", repos[0].Path)
	}
}

func TestWalk_skipsNonRepoDirs(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	if err := os.MkdirAll(filepath.Join(root, "not-a-repo/docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	repos, _ := Walk([]string{root})
	if len(repos) != 1 {
		t.Fatalf("want 1 repo, got %d (%+v)", len(repos), repos)
	}
}

func TestWalk_disambiguatesDuplicateNames(t *testing.T) {
	r1 := t.TempDir()
	r2 := t.TempDir()
	makeRepo(t, r1, "api")
	makeRepo(t, r2, "api")

	repos, err := Walk([]string{r1, r2})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("want 2 repos, got %d", len(repos))
	}

	names := []string{repos[0].Name, repos[1].Name}
	sort.Strings(names)
	// first appearance keeps plain name, second appearance gets <root-basename>/api
	// Order of Walk visits r1 first so repos[0].Name = "api" (plain).
	expected := []string{"api", filepath.Base(r2) + "/api"}
	sort.Strings(expected)
	if names[0] != expected[0] || names[1] != expected[1] {
		t.Fatalf("want %v, got %v", expected, names)
	}
}

func TestWalk_depth2Only(t *testing.T) {
	root := t.TempDir()
	// Depth 3 repo should NOT be discovered.
	deep := filepath.Join(root, "level1", "level2", "deep-repo")
	if err := os.MkdirAll(filepath.Join(deep, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Depth 2 repo SHOULD be discovered.
	makeRepo(t, root, "shallow")

	repos, _ := Walk([]string{root})
	if len(repos) != 1 || repos[0].Name != "shallow" {
		t.Fatalf("want [shallow], got %+v", repos)
	}
}
```

- [ ] **Step 2: Run test, expect compile failure**

Run: `go test ./internal/discover/...`
Expected: `undefined: Walk`, `undefined: Repo`.

- [ ] **Step 3: Implement `internal/discover/walk.go`**

File: `internal/discover/walk.go`
```go
package discover

import (
	"os"
	"path/filepath"
)

// Repo describes a discovered git repository.
type Repo struct {
	Name string // discovery-assigned; collides deduped as "<root-basename>/<name>"
	Path string // absolute path to the repo working tree
	Root string // configured root this repo was found under
}

// Walk scans each root at depth 2 looking for directories containing a .git
// entry. Repos are returned in the order roots and subdirs are visited.
// Collisions on bare Name get namespaced via the root's basename.
func Walk(roots []string) ([]Repo, error) {
	var out []Repo
	seen := map[string]bool{}

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			// Missing/unreadable roots are silently skipped; config layer
			// should validate before calling Walk if strict behavior wanted.
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidate := filepath.Join(root, entry.Name())
			if !hasDotGit(candidate) {
				continue
			}
			name := entry.Name()
			if seen[name] {
				name = filepath.Base(root) + "/" + entry.Name()
			}
			seen[entry.Name()] = true
			out = append(out, Repo{
				Name: name,
				Path: candidate,
				Root: root,
			})
		}
	}
	return out, nil
}

func hasDotGit(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}
```

Note: the walk is depth-2 (root/child → look for `.git`), matching the spec and the test `TestWalk_depth2Only`. Deeper nesting is ignored by design.

- [ ] **Step 4: Run test, expect PASS**

Run: `go test ./internal/discover/... -v`
Expected: all subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/discover
git commit -m "feat(discover): add depth-2 git repo walker with dedupe"
```

---

## Task 7: Cobra root command + global flags

Rewrite `cmd/ds/main.go` to build a Cobra root command with `--json` / `--human` / `--version` wired. Adds `github.com/spf13/cobra` dep; refreshes `vendorHash`.

**Files:**
- Modify: `cmd/ds/main.go`
- Create: `internal/cli/root.go`
- Create: `internal/cli/root_test.go`
- Modify: `go.mod`, `go.sum`, `flake.nix` (vendorHash)

- [ ] **Step 1: Add Cobra dep**

Run: `nix develop --command bash -c 'go get github.com/spf13/cobra && go mod tidy'`
Expected: cobra pinned in `go.mod`.

- [ ] **Step 2: Write failing test `internal/cli/root_test.go`**

File: `internal/cli/root_test.go`
```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewRootCmd_versionFlagPrints(t *testing.T) {
	var stdout bytes.Buffer
	cmd := NewRootCmd("v1.2.3")
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(stdout.String(), "v1.2.3") {
		t.Fatalf("stdout missing version: %q", stdout.String())
	}
}

func TestNewRootCmd_hasJSONAndHumanFlags(t *testing.T) {
	cmd := NewRootCmd("v0.0.0")
	if cmd.PersistentFlags().Lookup("json") == nil {
		t.Fatal("missing --json persistent flag")
	}
	if cmd.PersistentFlags().Lookup("human") == nil {
		t.Fatal("missing --human persistent flag")
	}
}

func TestNewRootCmd_hasReposSubcommand(t *testing.T) {
	cmd := NewRootCmd("v0.0.0")
	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "repos" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("repos subcommand not registered")
	}
}
```

- [ ] **Step 3: Run test, expect compile failure**

Run: `go test ./internal/cli/... -run TestNewRootCmd`
Expected: `undefined: NewRootCmd`.

- [ ] **Step 4: Implement `internal/cli/root.go`**

File: `internal/cli/root.go`
```go
package cli

import (
	"github.com/spf13/cobra"
)

// GlobalFlags holds persistent flags threaded through every subcommand.
type GlobalFlags struct {
	JSON  bool
	Human bool
}

// NewRootCmd constructs the `ds` root command with all v0.1 subcommands
// registered. The version string is injected by main (post-resolveVersion).
func NewRootCmd(version string) *cobra.Command {
	flags := &GlobalFlags{}

	root := &cobra.Command{
		Use:           "ds",
		Short:         "do-stuff: task-based multi-repo worktree manager",
		Version:       version,
		SilenceUsage:  true, // envelope already reports errors
		SilenceErrors: true,
	}
	root.SetVersionTemplate("ds {{.Version}}\n")

	root.PersistentFlags().BoolVar(&flags.JSON, "json", false, "force JSON envelope output")
	root.PersistentFlags().BoolVar(&flags.Human, "human", false, "force human-readable output")

	root.AddCommand(NewReposCmd(flags))

	return root
}
```

Note: `NewReposCmd` is defined in Task 8. If Step 6 runs before Task 8 exists, compilation fails — the plan is written such that `NewRootCmd` and `NewReposCmd` land in the same PR slice. If following tasks strictly in order, stub `NewReposCmd` in `root.go` temporarily OR skip straight to Task 8 before testing the root (recommended — the `TestNewRootCmd_hasReposSubcommand` test in Step 2 depends on it). To stay TDD-pure, add a placeholder in `internal/cli/root.go`:

```go
// tmp placeholder until Task 8 lands the real impl
func NewReposCmd(_ *GlobalFlags) *cobra.Command {
	return &cobra.Command{Use: "repos", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
}
```

Then Task 8 replaces this with the real thing. The plan assumes executors will do this — the placeholder is an intentional interim step, NOT a permanent no-op.

- [ ] **Step 5: Run test, expect PASS**

Run: `nix develop --command bash -c 'go test ./internal/cli/... -v'`
Expected: all subtests PASS (including the placeholder repos).

- [ ] **Step 6: Rewrite `cmd/ds/main.go`**

File: `cmd/ds/main.go`
```go
package main

import (
	"os"

	"github.com/jordangarrison/do-stuff/internal/cli"
)

func main() {
	root := cli.NewRootCmd(resolveVersion())
	if err := root.Execute(); err != nil {
		// Cobra returns errors bare. Actual envelope rendering happens
		// inside each RunE via cli.Render, so we only reach here on
		// unrecoverable framework errors. Exit non-zero.
		os.Exit(1)
	}
}
```

- [ ] **Step 7: Refresh vendorHash for cobra deps**

Edit `flake.nix`: set `vendorHash = pkgs.lib.fakeHash;`. Run `nix build .#default`. Paste real hash from the failure message.

Run: `nix build .#default && ./result/bin/ds --version`
Expected: `ds v0.0.0` (ldflag still wins over resolveVersion).

Run: `./result/bin/ds --help`
Expected: Cobra help text mentioning `repos` subcommand and `--json`/`--human` flags.

- [ ] **Step 8: Commit**

```bash
git add cmd/ds internal/cli/root.go internal/cli/root_test.go go.mod go.sum flake.nix
git commit -m "feat(cli): wire cobra root with --json/--human flags"
```

---

## Task 8: `ds repos` command + golden test

Real `repos.go` reads config via `internal/config`, walks roots via `internal/discover`, emits the success envelope. Fails with `config_error` when `repo_roots` is empty. Replaces the Task 7 placeholder.

**Files:**
- Create: `internal/cli/repos.go` (real impl; overwrite placeholder inside `root.go`, remove the placeholder there)
- Create: `internal/cli/repos_test.go`
- Modify: `internal/cli/root.go` (remove placeholder)

- [ ] **Step 1: Remove placeholder from `internal/cli/root.go`**

Delete the temporary `func NewReposCmd(...)` block added in Task 7 Step 4. `NewRootCmd` still references `NewReposCmd(flags)`; the real implementation is in the new file below.

- [ ] **Step 2: Write failing test `internal/cli/repos_test.go`**

File: `internal/cli/repos_test.go`
```go
package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes a config.yaml containing the given repo_roots and returns its path.
func writeConfig(t *testing.T, roots []string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	body := "repo_roots:\n"
	for _, r := range roots {
		body += "  - " + r + "\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRepos_successGoldenEnvelope(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "api", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfig(t, []string{root})

	var stdout, stderr bytes.Buffer
	code := runReposForTest(reposOpts{
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})

	if code != 0 {
		t.Fatalf("want exit 0, got %d: stderr=%s", code, stderr.String())
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if !env.OK || env.Command != "ds.repos" {
		t.Fatalf("bad envelope: %+v", env)
	}
	data, _ := env.Data.(map[string]any)
	if data == nil {
		t.Fatal("missing data")
	}
	repos, _ := data["repos"].([]any)
	if len(repos) != 1 {
		t.Fatalf("want 1 repo, got %d (data=%+v)", len(repos), data)
	}
}

func TestRepos_emptyRepoRootsReturnsConfigError(t *testing.T) {
	cfgPath := writeConfig(t, nil)

	var stdout, stderr bytes.Buffer
	code := runReposForTest(reposOpts{
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})

	if code != 8 {
		t.Fatalf("want exit 8 (config_error), got %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if env.OK || env.Error == nil {
		t.Fatalf("bad envelope: %+v", env)
	}
	if env.Error.Code != "config_error" {
		t.Fatalf("want config_error, got %s", env.Error.Code)
	}
}
```

- [ ] **Step 3: Run test, expect compile failure**

Run: `go test ./internal/cli/... -run TestRepos`
Expected: `undefined: runReposForTest`, `undefined: reposOpts`.

- [ ] **Step 4: Implement `internal/cli/repos.go`**

File: `internal/cli/repos.go`
```go
package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/jordangarrison/do-stuff/internal/config"
	"github.com/jordangarrison/do-stuff/internal/discover"
	"github.com/jordangarrison/do-stuff/internal/errs"
)

// NewReposCmd builds `ds repos`. Reads config, walks roots, emits envelope.
func NewReposCmd(flags *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "repos",
		Short: "list configured repo roots and discovered repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := DetectMode(DetectOpts{
				IsTerminal: IsStdoutTerminal(),
				JSON:       flags.JSON,
				Human:      flags.Human,
			})
			code := runReposForTest(reposOpts{
				ConfigPath: config.DefaultPath(),
				Mode:       mode,
				Stdout:     os.Stdout,
				Stderr:     os.Stderr,
			})
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
}

// reposOpts isolates the logic from global state so tests can drive it.
type reposOpts struct {
	ConfigPath string
	Mode       Mode
	Stdout     io.Writer
	Stderr     io.Writer
}

// runReposForTest is the testable core of `ds repos`. Returns the exit code
// (also written via Render). Kept exported-in-test via the same package.
func runReposForTest(o reposOpts) int {
	cfg, err := config.Load(o.ConfigPath)
	if err != nil {
		return Render(RenderOpts{
			Command: "ds.repos",
			Err:     err,
			Stdout:  o.Stdout,
			Stderr:  o.Stderr,
			Mode:    o.Mode,
		})
	}

	if len(cfg.RepoRoots) == 0 {
		return Render(RenderOpts{
			Command: "ds.repos",
			Err: &errs.TaskError{
				Code:    errs.ConfigError,
				Message: "repo_roots is empty; set it in " + o.ConfigPath,
				Details: map[string]any{"config_path": o.ConfigPath},
			},
			Stdout: o.Stdout,
			Stderr: o.Stderr,
			Mode:   o.Mode,
		})
	}

	repos, err := discover.Walk(cfg.RepoRoots)
	if err != nil {
		return Render(RenderOpts{
			Command: "ds.repos",
			Err:     err,
			Stdout:  o.Stdout,
			Stderr:  o.Stderr,
			Mode:    o.Mode,
		})
	}

	return Render(RenderOpts{
		Command: "ds.repos",
		Data:    marshalReposData(repos, cfg.RepoRoots),
		Stdout:  o.Stdout,
		Stderr:  o.Stderr,
		Mode:    o.Mode,
	})
}

func marshalReposData(repos []discover.Repo, roots []string) map[string]any {
	r := make([]map[string]any, 0, len(repos))
	for _, repo := range repos {
		r = append(r, map[string]any{
			"name": repo.Name,
			"path": repo.Path,
			"root": repo.Root,
		})
	}
	return map[string]any{
		"repos": r,
		"roots": roots,
	}
}
```

- [ ] **Step 5: Run tests, expect PASS**

Run: `nix develop --command bash -c 'go test ./... -v'`
Expected: all tests across all packages pass.

- [ ] **Step 6: End-to-end smoke test against real binary**

Build: `nix build .#default`

With no config: `HOME=/tmp/empty-home XDG_CONFIG_HOME= ./result/bin/ds repos --json`
Expected: exit code 8, envelope with `"code": "config_error"`.

With a populated config:
```bash
mkdir -p /tmp/ds-e2e/{root,root/foo/.git,config/do-stuff}
cat > /tmp/ds-e2e/config/do-stuff/config.yaml <<YAML
repo_roots:
  - /tmp/ds-e2e/root
YAML
XDG_CONFIG_HOME=/tmp/ds-e2e/config ./result/bin/ds repos --json
```
Expected: exit 0, envelope with `"ok": true` and `repos` list containing `foo`.

Cleanup: `rm -rf /tmp/ds-e2e`.

- [ ] **Step 7: Refresh `vendorHash` if dependency tree changed**

Run: `nix build .#default`
If it succeeds, skip. If it fails with hash mismatch, set `vendorHash = pkgs.lib.fakeHash;`, rebuild, paste real hash, rebuild again.

- [ ] **Step 8: Commit**

```bash
git add internal/cli/repos.go internal/cli/repos_test.go internal/cli/root.go flake.nix go.mod go.sum
git commit -m "feat(cli): add ds repos command emitting envelope"
```

---

## Task 9: Final acceptance + README update

No new code. Run every v0.1-slice-1 surface end-to-end, update README to mention the YAML config file, and tag the slice.

**Files:**
- Modify: `README.md`

- [ ] **Step 1: `go test ./...` green**

Run: `nix develop --command bash -c 'go test ./... -v'`
Expected: all packages pass.

- [ ] **Step 2: `nix flake check` green**

Run: `nix flake check --print-build-logs`
Expected: package builds + hooks + tests all pass.

- [ ] **Step 3: Pre-commit on the full tree**

Run: `nix develop --command bash -c 'pre-commit run --all-files'`
Expected: every hook reports `Passed` or `Skipped`.

- [ ] **Step 4: GoReleaser snapshot still builds**

Run: `nix develop --command bash -c 'goreleaser release --snapshot --clean'`
Expected: four platform binaries + `checksums.txt` in `dist/`. Clean with `rm -rf dist/`.

- [ ] **Step 5: `ds --version` via all three install paths**

```bash
# 1. local go build
nix develop --command bash -c 'go build -o /tmp/ds-go ./cmd/ds && /tmp/ds-go --version'
# expected: ds dev

# 2. nix build
nix build .#default && ./result/bin/ds --version
# expected: ds v0.0.0   (ldflag; will be real version after v0.1 tag)

# 3. nix run
nix run .# -- --version
# expected: ds v0.0.0
```

- [ ] **Step 6: Update `README.md`**

Replace `README.md` contents with:

````markdown
# do-stuff

An opinionated toolkit for spinning up and resuming multi-repo work organized as tasks. Ships a `ds` CLI binary and a set of Claude skills that drive it.

Status: v0.1 slice 1 — `ds repos` wired, Cobra foundation landed. No task creation / resume yet (that's slice 2).

## Install

### Nix (primary)

```
nix profile install github:jordangarrison/do-stuff
```

### Go

```
go install github.com/jordangarrison/do-stuff/cmd/ds@latest
```

## Configure

`ds` reads YAML from `$XDG_CONFIG_HOME/do-stuff/config.yaml` (default `~/.config/do-stuff/config.yaml`):

```yaml
# Paths can use ~ or $HOME; expanded at load time.
tasks_dir: ~/.do-stuff
repo_roots:
  - ~/dev/work
  - ~/dev/personal
tmux_prefix: task-
default_base: main
default_type: feat
start_tmux: true
```

`repo_roots` is required — everything else has defaults. `ds repos` errors cleanly with `code: config_error` until it's set.

## Development

```
nix develop            # or `direnv allow` once
go build ./...
go test ./...
```

## License

MIT
````

- [ ] **Step 7: Commit**

```bash
git add README.md
git commit -m "docs: describe yaml config + v0.1 slice 1 status"
```

- [ ] **Step 8: Summarize final commit graph**

Run: `git log --oneline -15`
Expected (order may vary):
```
docs: describe yaml config + v0.1 slice 1 status
feat(cli): add ds repos command emitting envelope
feat(cli): wire cobra root with --json/--human flags
feat(discover): add depth-2 git repo walker with dedupe
feat(config): add yaml loader with xdg + tilde expansion
feat(cli): add envelope renderer with tty detection
feat(errs): add TaskError + exit-code mapping per spec
refactor: hoist ds derivation into shared mkDs factory
fix: resolve ds version from buildinfo when ldflags absent
```

- [ ] **Step 9: Do NOT push yet**

Stop here. Push + PR creation is a separate, user-authorized step per standing rules. Report success back to user with the `git log --oneline` summary.

---

## Self-review appendix (agent: check before handing back)

Before reporting completion, agent walks through this list against the final state:

1. **Spec coverage:** Each v0.1 milestone bullet maps to at least one task.
   - Cobra wiring → Task 7.
   - isatty detection → Task 4 (`DetectMode` + `IsStdoutTerminal`).
   - Envelope rendering → Task 4.
   - `ds repos` → Task 8.
   - Full error-code table → Task 3 (all 13 codes + ExitCode).
   - Bootstrap carry-over (version resolver) → Task 1.
   - Overlay refactor → Task 2.
   - Real `vendorHash` → Tasks 4, 5, 7.
   - Skills + install.sh → OUT OF SCOPE (slice 3).
   - `new`/`list`/`pick`/`attach` → OUT OF SCOPE (slice 2).

2. **Deviations documented:**
   - YAML instead of TOML (Task 5 header).
   - Version sentinel is `"dev"` string (Task 1).

3. **No placeholders:** Scanned — no `TODO`, `TBD`, `implement later` inside task bodies. The intentional Task 7 placeholder for `NewReposCmd` is documented as intentional and removed in Task 8.

4. **Type consistency:**
   - `errs.TaskError`, `errs.Code`, `errs.ConfigError` used consistently.
   - `cli.Envelope`, `cli.Render`, `cli.RenderOpts` stable across tasks.
   - `config.Config`, `config.Load`, `config.DefaultPath` stable.
   - `discover.Repo`, `discover.Walk` stable.

5. **Every step with code has code.** Every step with a command has a command and expected output. No prose-only "add handling" steps.
