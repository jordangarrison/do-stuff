package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type listEnvelope struct {
	OK      bool     `json:"ok"`
	Command string   `json:"command"`
	Data    listData `json:"data,omitempty"`
}

type listData struct {
	Tasks []listTask `json:"tasks"`
}

type listTask struct {
	Slug         string   `json:"slug"`
	Type         string   `json:"type"`
	Ticket       string   `json:"ticket,omitempty"`
	Branch       string   `json:"branch"`
	Repos        []string `json:"repos"`
	Session      string   `json:"session,omitempty"`
	SessionState string   `json:"session_state"`
	CreatedAt    string   `json:"created_at"`
}

func writeTaskFile(t *testing.T, tasksDir, slug, body string) {
	t.Helper()
	dir := filepath.Join(tasksDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".task.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeListConfig(t *testing.T, tasksDir string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	body := "tasks_dir: " + tasksDir + "\nstart_tmux: false\nrepo_roots:\n  - " + t.TempDir() + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestList_emptyTasksDir(t *testing.T) {
	tasksDir := t.TempDir()
	cfgPath := writeListConfig(t, tasksDir)

	var stdout, stderr bytes.Buffer
	code := runList(listOpts{
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var env listEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if !env.OK || env.Command != "ds.list" {
		t.Fatalf("envelope: %+v", env)
	}
	if env.Data.Tasks == nil {
		t.Fatal("tasks must be [] not null")
	}
	if len(env.Data.Tasks) != 0 {
		t.Fatalf("want 0 tasks, got %d", len(env.Data.Tasks))
	}
}

func TestList_listsValidTasks(t *testing.T) {
	tasksDir := t.TempDir()
	writeTaskFile(t, tasksDir, "alpha", `{
		"slug": "alpha",
		"type": "feat",
		"ticket": "INFRA-1",
		"branch": "feat/infra-1-alpha",
		"base": "main",
		"tmux_session": "",
		"repos": [
			{"name": "api", "path": "/abs/api", "worktree": "api"},
			{"name": "web", "path": "/abs/web", "worktree": "web"}
		],
		"created_at": "2026-04-23T10:00:00Z"
	}`)
	cfgPath := writeListConfig(t, tasksDir)

	var stdout, stderr bytes.Buffer
	code := runList(listOpts{
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var env listEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if len(env.Data.Tasks) != 1 {
		t.Fatalf("want 1, got %d", len(env.Data.Tasks))
	}
	got := env.Data.Tasks[0]
	if got.Slug != "alpha" || got.Type != "feat" || got.Ticket != "INFRA-1" {
		t.Fatalf("task: %+v", got)
	}
	if len(got.Repos) != 2 || got.Repos[0] != "api" || got.Repos[1] != "web" {
		t.Fatalf("repos: %+v", got.Repos)
	}
	if got.SessionState != "absent" {
		t.Fatalf("want absent (no session), got %q", got.SessionState)
	}
}

func TestList_skipsMalformedTaskJSON(t *testing.T) {
	tasksDir := t.TempDir()
	writeTaskFile(t, tasksDir, "bad", "{not json")
	writeTaskFile(t, tasksDir, "good", `{
		"slug": "good",
		"type": "feat",
		"branch": "feat/good",
		"base": "main",
		"repos": [{"name": "api", "path": "/abs/api", "worktree": "api"}],
		"created_at": "2026-04-23T10:00:00Z"
	}`)
	cfgPath := writeListConfig(t, tasksDir)

	var stdout, stderr bytes.Buffer
	code := runList(listOpts{
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if code != 0 {
		t.Fatalf("want success despite malformed file, got exit %d stderr=%s", code, stderr.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("warn:")) {
		t.Fatalf("expected warn line on stderr, got %q", stderr.String())
	}
	var env listEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if len(env.Data.Tasks) != 1 || env.Data.Tasks[0].Slug != "good" {
		t.Fatalf("tasks: %+v", env.Data.Tasks)
	}
}

func TestList_missingTasksDirIsEmpty(t *testing.T) {
	cfgPath := writeListConfig(t, "/nonexistent/path/should/not/exist")
	var stdout, stderr bytes.Buffer
	code := runList(listOpts{
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if code != 0 {
		t.Fatalf("want 0 for missing tasks_dir, got %d", code)
	}
	var env listEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json: %v", err)
	}
	if env.Data.Tasks == nil {
		t.Fatal("tasks must be [] not null")
	}
}
