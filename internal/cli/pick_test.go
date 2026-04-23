package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/task"
)

func writePickConfig(t *testing.T, tasksDir string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "tasks_dir: " + tasksDir + "\n" +
		"repo_roots: [/tmp]\n" +
		"tmux_prefix: ds-test-\n" +
		"default_base: main\n" +
		"default_type: feat\n" +
		"start_tmux: true\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func seedPickTask(t *testing.T, tasksDir, slug, ticket, session string, repos []string) {
	t.Helper()
	taskDir := filepath.Join(tasksDir, slug)
	for _, r := range repos {
		if err := os.MkdirAll(filepath.Join(taskDir, r), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	refs := make([]task.RepoRef, 0, len(repos))
	for _, r := range repos {
		refs = append(refs, task.RepoRef{Name: r, Path: "/src/" + r, Worktree: r})
	}
	if err := task.Write(taskDir, &task.Task{
		Slug:        slug,
		Type:        "feat",
		Ticket:      ticket,
		Branch:      "feat/" + slug,
		Base:        "main",
		Repos:       refs,
		TmuxSession: session,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPick_previewPrintsTaskBlock(t *testing.T) {
	tasksDir := t.TempDir()
	seedPickTask(t, tasksDir, "alpha", "INFRA-1", "ds-test-alpha", []string{"api", "web"})
	cfg := writePickConfig(t, tasksDir)

	var out, errb bytes.Buffer
	code := runPick(pickOpts{
		PreviewSlug: "alpha",
		ConfigPath:  cfg,
		Mode:        ModeHuman,
		Stdout:      &out,
		Stderr:      &errb,
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	body := out.String()
	for _, want := range []string{
		"slug:    alpha",
		"type:    feat",
		"ticket:  INFRA-1",
		"branch:  feat/alpha",
		"base:    main",
		"repos:   api, web",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("preview missing %q:\n%s", want, body)
		}
	}
}

func TestPick_previewMissingTaskSoftFails(t *testing.T) {
	tasksDir := t.TempDir()
	cfg := writePickConfig(t, tasksDir)

	var out, errb bytes.Buffer
	code := runPick(pickOpts{
		PreviewSlug: "ghost",
		ConfigPath:  cfg,
		Mode:        ModeHuman,
		Stdout:      &out,
		Stderr:      &errb,
	})
	if code != 0 {
		t.Fatalf("preview must not hard-fail fzf; code=%d", code)
	}
	if !strings.Contains(errb.String(), "ghost") {
		t.Fatalf("expected stderr note about missing slug, got: %s", errb.String())
	}
}

func TestPick_ttyExecsDsAttach(t *testing.T) {
	tasksDir := t.TempDir()
	seedPickTask(t, tasksDir, "alpha", "", "ds-test-alpha", []string{"api"})
	cfg := writePickConfig(t, tasksDir)

	var out, errb bytes.Buffer
	var (
		gotArgv0 string
		gotArgv  []string
	)
	code := runPick(pickOpts{
		ConfigPath: cfg,
		Mode:       ModeHuman,
		Stdout:     &out,
		Stderr:     &errb,
		LookupFn:   func(bin string) (string, error) { return "/usr/bin/" + bin, nil },
		SelectorFn: func(slugs []string) (string, error) { return "alpha", nil },
		ExecFn: func(argv0 string, argv []string, env []string) error {
			gotArgv0, gotArgv = argv0, argv
			return nil
		},
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	if !strings.HasSuffix(gotArgv0, "ds") && !strings.HasSuffix(gotArgv0, "ds.test") {
		// Argv0 is os.Args[0]; in go test it's the test binary.
		// We only assert the tail here.
		_ = gotArgv0
	}
	if len(gotArgv) < 3 || gotArgv[len(gotArgv)-2] != "attach" || gotArgv[len(gotArgv)-1] != "alpha" {
		t.Fatalf("argv tail expected [.. attach alpha], got %v", gotArgv)
	}
}

func TestPick_pipedEmitsEnvelopeNoExec(t *testing.T) {
	tasksDir := t.TempDir()
	seedPickTask(t, tasksDir, "alpha", "INFRA-1", "ds-test-alpha", []string{"api", "web"})
	cfg := writePickConfig(t, tasksDir)

	var out, errb bytes.Buffer
	execCalls := 0
	code := runPick(pickOpts{
		ConfigPath: cfg,
		Mode:       ModeJSON,
		Stdout:     &out,
		Stderr:     &errb,
		LookupFn:   func(bin string) (string, error) { return "/usr/bin/" + bin, nil },
		SelectorFn: func(slugs []string) (string, error) { return "alpha", nil },
		ExecFn:     func(string, []string, []string) error { execCalls++; return nil },
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	if execCalls != 0 {
		t.Fatalf("piped path must not exec; calls=%d", execCalls)
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	raw, _ := json.Marshal(env.Data)
	var d PickData
	_ = json.Unmarshal(raw, &d)
	if d.Slug != "alpha" || d.Ticket != "INFRA-1" || len(d.Repos) != 2 || d.Session != "ds-test-alpha" {
		t.Fatalf("data: %+v", d)
	}
	if d.AttachCommand != "tmux attach -t ds-test-alpha" {
		t.Fatalf("attach_command: %q", d.AttachCommand)
	}
}

func TestPick_fzfMissingEmitsPickUnavailable(t *testing.T) {
	tasksDir := t.TempDir()
	seedPickTask(t, tasksDir, "alpha", "", "", []string{"api"})
	cfg := writePickConfig(t, tasksDir)

	var out, errb bytes.Buffer
	code := runPick(pickOpts{
		ConfigPath: cfg,
		Mode:       ModeJSON,
		Stdout:     &out, Stderr: &errb,
		LookupFn: func(bin string) (string, error) {
			if bin == "fzf" {
				return "", exec.ErrNotFound
			}
			return "/usr/bin/" + bin, nil
		},
	})
	if code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	var env Envelope
	_ = json.Unmarshal(out.Bytes(), &env)
	if env.OK || env.Error.Code != errs.PickUnavailable {
		t.Fatalf("envelope: %+v", env)
	}
}

func TestPick_emptyTasksDirEmitsTaskNotFound(t *testing.T) {
	tasksDir := t.TempDir() // intentionally empty
	cfg := writePickConfig(t, tasksDir)

	var out, errb bytes.Buffer
	code := runPick(pickOpts{
		ConfigPath: cfg,
		Mode:       ModeJSON,
		Stdout:     &out, Stderr: &errb,
	})
	if code != 9 {
		t.Fatalf("want 9, got %d", code)
	}
	var env Envelope
	_ = json.Unmarshal(out.Bytes(), &env)
	if env.OK || env.Error.Code != errs.TaskNotFound {
		t.Fatalf("envelope: %+v", env)
	}
}

func TestPick_pipedWarnsOnUnreadableTask(t *testing.T) {
	tasksDir := t.TempDir()
	seedPickTask(t, tasksDir, "alpha", "", "ds-test-alpha", []string{"api"})

	// Seed a broken task: directory with a malformed .task.json so task.Load fails.
	brokenDir := filepath.Join(tasksDir, "broken")
	if err := os.MkdirAll(brokenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	brokenMeta := filepath.Join(brokenDir, task.MetadataFile)
	if err := os.WriteFile(brokenMeta, []byte("["), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := writePickConfig(t, tasksDir)

	var out, errb bytes.Buffer
	code := runPick(pickOpts{
		ConfigPath: cfg,
		Mode:       ModeJSON,
		Stdout:     &out, Stderr: &errb,
		LookupFn:   func(bin string) (string, error) { return "/usr/bin/" + bin, nil },
		SelectorFn: func(slugs []string) (string, error) { return "alpha", nil },
		ExecFn:     func(string, []string, []string) error { return nil },
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, out.String())
	}
	raw, _ := json.Marshal(env.Data)
	var d PickData
	_ = json.Unmarshal(raw, &d)
	if d.Slug != "alpha" {
		t.Fatalf("expected envelope for alpha, got %+v", d)
	}
	if !strings.Contains(errb.String(), "warn:") {
		t.Fatalf("expected 'warn:' in stderr, got: %s", errb.String())
	}
	if !strings.Contains(errb.String(), brokenDir) {
		t.Fatalf("expected broken task path %q in stderr, got: %s", brokenDir, errb.String())
	}
}

func TestPick_selectionCancelledPiped(t *testing.T) {
	tasksDir := t.TempDir()
	seedPickTask(t, tasksDir, "alpha", "", "", []string{"api"})
	cfg := writePickConfig(t, tasksDir)

	var out, errb bytes.Buffer
	code := runPick(pickOpts{
		ConfigPath: cfg,
		Mode:       ModeJSON,
		Stdout:     &out, Stderr: &errb,
		LookupFn:   func(bin string) (string, error) { return "/usr/bin/" + bin, nil },
		SelectorFn: func(slugs []string) (string, error) { return "", errPickCancelled },
	})
	if code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	var env Envelope
	_ = json.Unmarshal(out.Bytes(), &env)
	if env.OK || env.Error.Code != errs.InvalidArgs {
		t.Fatalf("envelope: %+v", env)
	}
}
