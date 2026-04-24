package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/task"
)

func writeFinishConfig(t *testing.T, tasksDir string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "tasks_dir: " + tasksDir + "\n" +
		"repo_roots: [/tmp]\n" +
		"tmux_prefix: ds-test-\n" +
		"default_base: main\n" +
		"default_type: feat\n" +
		"start_tmux: false\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFinish_pipedEnvelope(t *testing.T) {
	tasksDir := t.TempDir()
	cfg := writeFinishConfig(t, tasksDir)

	var out, errb bytes.Buffer
	var gotParams task.FinishParams
	code := runFinish(finishOpts{
		Slug:         "demo",
		Force:        true,
		KeepBranches: false,
		ConfigPath:   cfg,
		Mode:         ModeJSON,
		Stdout:       &out,
		Stderr:       &errb,
		FinishFn: func(p task.FinishParams) (*task.FinishResult, error) {
			gotParams = p
			return &task.FinishResult{
				Task:             &task.Task{Slug: "demo"},
				RemovedWorktrees: []string{"api", "web"},
				KilledSession:    "ds-test-demo",
				BranchesKept:     false,
			}, nil
		},
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errb.String())
	}
	if gotParams.Slug != "demo" || !gotParams.Force || gotParams.KeepBranches || gotParams.TasksDir != tasksDir {
		t.Fatalf("params=%+v", gotParams)
	}

	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out.String())
	}
	if !env.OK || env.Command != "ds.finish" {
		t.Fatalf("envelope: %+v", env)
	}
	raw, _ := json.Marshal(env.Data)
	var d FinishData
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if d.Slug != "demo" || d.KilledSession != "ds-test-demo" || d.BranchesKept {
		t.Fatalf("data: %+v", d)
	}
	if len(d.RemovedWorktrees) != 2 || d.RemovedWorktrees[0] != "api" || d.RemovedWorktrees[1] != "web" {
		t.Fatalf("removed_worktrees: %v", d.RemovedWorktrees)
	}
}

func TestFinish_killedSessionOmitEmpty(t *testing.T) {
	tasksDir := t.TempDir()
	cfg := writeFinishConfig(t, tasksDir)

	var out bytes.Buffer
	code := runFinish(finishOpts{
		Slug:       "demo",
		ConfigPath: cfg,
		Mode:       ModeJSON,
		Stdout:     &out,
		Stderr:     &bytes.Buffer{},
		FinishFn: func(p task.FinishParams) (*task.FinishResult, error) {
			return &task.FinishResult{
				Task:             &task.Task{Slug: "demo"},
				RemovedWorktrees: []string{},
				KilledSession:    "",
				BranchesKept:     false,
			}, nil
		},
	})
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if bytes.Contains(out.Bytes(), []byte("killed_session")) {
		t.Fatalf("killed_session must be omitted when empty, got: %s", out.String())
	}
}

func TestFinish_invalidSlug(t *testing.T) {
	var out, errb bytes.Buffer
	code := runFinish(finishOpts{
		Slug:       "Bad Slug!",
		ConfigPath: writeFinishConfig(t, t.TempDir()),
		Mode:       ModeJSON,
		Stdout:     &out,
		Stderr:     &errb,
		FinishFn: func(p task.FinishParams) (*task.FinishResult, error) {
			t.Fatal("FinishFn must not run on invalid slug")
			return nil, nil
		},
	})
	if code != 2 {
		t.Fatalf("code=%d want 2", code)
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.OK {
		t.Fatal("envelope ok=true; want false")
	}
	if env.Error == nil || env.Error.Code != errs.InvalidArgs {
		t.Fatalf("error: %+v", env.Error)
	}
}

func TestFinish_propagatesOrchestratorError(t *testing.T) {
	var out, errb bytes.Buffer
	code := runFinish(finishOpts{
		Slug:       "demo",
		ConfigPath: writeFinishConfig(t, t.TempDir()),
		Mode:       ModeJSON,
		Stdout:     &out,
		Stderr:     &errb,
		FinishFn: func(p task.FinishParams) (*task.FinishResult, error) {
			return nil, &errs.TaskError{
				Code:    errs.WorktreeDirty,
				Message: "dirty",
				Details: map[string]any{"repo": "api"},
			}
		},
	})
	if code != 7 {
		t.Fatalf("code=%d want 7", code)
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.OK || env.Error == nil || env.Error.Code != errs.WorktreeDirty {
		t.Fatalf("envelope: %+v", env)
	}
}
