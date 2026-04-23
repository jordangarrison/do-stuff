package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/task"
)

func writeAttachConfig(t *testing.T, tasksDir string) string {
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

func seedAttachTask(t *testing.T, tasksDir, slug, session string) {
	t.Helper()
	taskDir := filepath.Join(tasksDir, slug)
	if err := os.MkdirAll(filepath.Join(taskDir, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := task.Write(taskDir, &task.Task{
		Slug:        slug,
		Type:        "feat",
		Branch:      "feat/" + slug,
		Base:        "main",
		Repos:       []task.RepoRef{{Name: "api", Path: "/src/api", Worktree: "api"}},
		TmuxSession: session,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAttach_pipedEmitsEnvelopeNoExec(t *testing.T) {
	tasksDir := t.TempDir()
	seedAttachTask(t, tasksDir, "slug-a", "ds-test-slug-a")
	cfg := writeAttachConfig(t, tasksDir)

	var out, errb bytes.Buffer
	var execCalls int
	code := runAttach(attachOpts{
		Slug:       "slug-a",
		ConfigPath: cfg,
		Mode:       ModeJSON,
		Stdout:     &out,
		Stderr:     &errb,
		AttachFn: func(task.AttachParams) (*task.AttachResult, error) {
			return &task.AttachResult{
				Task:         &task.Task{Slug: "slug-a"},
				SessionName:  "ds-test-slug-a",
				WasRecreated: false,
			}, nil
		},
		ExecFn: func(argv0 string, argv []string, env []string) error {
			execCalls++
			return nil
		},
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	if execCalls != 0 {
		t.Fatalf("ExecFn must not run on piped path, calls=%d", execCalls)
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out.String())
	}
	if !env.OK || env.Command != "ds.attach" {
		t.Fatalf("envelope: %+v", env)
	}
	// Data shape: slug/session/was_recreated/attach_command
	raw, _ := json.Marshal(env.Data)
	var d AttachData
	_ = json.Unmarshal(raw, &d)
	if d.Slug != "slug-a" || d.Session != "ds-test-slug-a" || d.WasRecreated {
		t.Fatalf("data: %+v", d)
	}
	if d.AttachCommand != "tmux attach -t ds-test-slug-a" {
		t.Fatalf("attach_command: %q", d.AttachCommand)
	}
}

func TestAttach_ttyExecsTmux(t *testing.T) {
	tasksDir := t.TempDir()
	seedAttachTask(t, tasksDir, "slug-b", "ds-test-slug-b")
	cfg := writeAttachConfig(t, tasksDir)

	var out, errb bytes.Buffer
	var (
		gotArgv0 string
		gotArgv  []string
	)
	code := runAttach(attachOpts{
		Slug:       "slug-b",
		ConfigPath: cfg,
		Mode:       ModeHuman,
		Stdout:     &out,
		Stderr:     &errb,
		AttachFn: func(task.AttachParams) (*task.AttachResult, error) {
			return &task.AttachResult{
				Task:         &task.Task{Slug: "slug-b"},
				SessionName:  "ds-test-slug-b",
				WasRecreated: true,
			}, nil
		},
		ExecFn: func(argv0 string, argv []string, env []string) error {
			gotArgv0, gotArgv = argv0, argv
			return nil
		},
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	if gotArgv0 == "" || len(gotArgv) < 4 {
		t.Fatalf("exec not called correctly: argv0=%q argv=%v", gotArgv0, gotArgv)
	}
	// argv[0] mirrors argv0; the tmux args trail.
	wantTail := []string{"attach", "-t", "ds-test-slug-b"}
	for i := range wantTail {
		if gotArgv[len(gotArgv)-len(wantTail)+i] != wantTail[i] {
			t.Fatalf("argv tail: want %v, got %v", wantTail, gotArgv)
		}
	}
}

func TestAttach_invalidSlug(t *testing.T) {
	var out, errb bytes.Buffer
	code := runAttach(attachOpts{
		Slug:   "Bad/Slug",
		Mode:   ModeJSON,
		Stdout: &out, Stderr: &errb,
	})
	if code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	var env Envelope
	_ = json.Unmarshal(out.Bytes(), &env)
	if env.OK || env.Error == nil || env.Error.Code != errs.InvalidArgs {
		t.Fatalf("envelope: %+v", env)
	}
}

func TestAttach_propagatesAttachError(t *testing.T) {
	cfg := writeAttachConfig(t, t.TempDir())
	var out, errb bytes.Buffer
	code := runAttach(attachOpts{
		Slug:       "ok-slug",
		ConfigPath: cfg,
		Mode:       ModeJSON,
		Stdout:     &out,
		Stderr:     &errb,
		AttachFn: func(task.AttachParams) (*task.AttachResult, error) {
			return nil, &errs.TaskError{Code: errs.TmuxSessionMissing, Message: "no session"}
		},
	})
	if code != 6 {
		t.Fatalf("want 6, got %d", code)
	}
	var env Envelope
	_ = json.Unmarshal(out.Bytes(), &env)
	if env.OK || env.Error.Code != errs.TmuxSessionMissing {
		t.Fatalf("envelope: %+v", env)
	}
}
