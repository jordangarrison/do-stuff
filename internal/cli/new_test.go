package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jordangarrison/do-stuff/internal/testutil"
)

func writeConfigWithRoot(t *testing.T, tasksDir, repoRoot string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	body := "tasks_dir: " + tasksDir + "\nstart_tmux: false\nrepo_roots:\n  - " + repoRoot + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

type newSuccessData struct {
	Slug          string `json:"slug"`
	Path          string `json:"path"`
	Branch        string `json:"branch"`
	Base          string `json:"base"`
	Ticket        string `json:"ticket,omitempty"`
	TmuxSession   string `json:"tmux_session,omitempty"`
	AttachCommand string `json:"attach_command,omitempty"`
	Repos         []struct {
		Name         string `json:"name"`
		WorktreePath string `json:"worktree_path"`
		BranchState  string `json:"branch_state"`
	} `json:"repos"`
}

type newEnvelope struct {
	OK      bool           `json:"ok"`
	Command string         `json:"command"`
	Data    newSuccessData `json:"data,omitempty"`
	Error   *struct {
		Code string `json:"code"`
	} `json:"error,omitempty"`
}

func TestNew_successEnvelope(t *testing.T) {
	// Build a "root" holding one fixture repo named "api".
	root := t.TempDir()
	api := testutil.InitFixtureRepo(t)
	// Move the fixture repo into <root>/api so discovery finds it.
	target := filepath.Join(root, "api")
	if err := os.Rename(api, target); err != nil {
		t.Fatal(err)
	}

	tasksDir := t.TempDir()
	cfgPath := writeConfigWithRoot(t, tasksDir, root)

	var stdout, stderr bytes.Buffer
	code := runNew(newOpts{
		Slug:       "demo",
		Type:       "feat",
		Repos:      []string{"api"},
		Base:       "main",
		NoTmux:     true,
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	var env newEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if !env.OK || env.Command != "ds.new" {
		t.Fatalf("envelope: %+v", env)
	}
	if env.Data.Slug != "demo" || env.Data.Branch != "feat/demo" {
		t.Fatalf("data: %+v", env.Data)
	}
	if len(env.Data.Repos) != 1 || env.Data.Repos[0].BranchState != "created" {
		t.Fatalf("repos: %+v", env.Data.Repos)
	}
	if env.Data.TmuxSession != "" {
		t.Fatalf("NoTmux: tmux_session should be empty, got %q", env.Data.TmuxSession)
	}
	// Ensure .task.json landed.
	if _, err := os.Stat(filepath.Join(tasksDir, "demo", ".task.json")); err != nil {
		t.Fatalf(".task.json missing: %v", err)
	}
}

func TestNew_repoNotFound(t *testing.T) {
	root := t.TempDir()
	tasksDir := t.TempDir()
	cfgPath := writeConfigWithRoot(t, tasksDir, root)

	var stdout, stderr bytes.Buffer
	code := runNew(newOpts{
		Slug:       "x",
		Type:       "feat",
		Repos:      []string{"nope"},
		Base:       "main",
		NoTmux:     true,
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if code != 3 {
		t.Fatalf("want exit 3 (repo_not_found), got %d", code)
	}
	var env newEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if env.OK || env.Error == nil || env.Error.Code != "repo_not_found" {
		t.Fatalf("envelope: %+v", env)
	}
}

func TestNew_invalidType(t *testing.T) {
	root := t.TempDir()
	tasksDir := t.TempDir()
	cfgPath := writeConfigWithRoot(t, tasksDir, root)

	var stdout, stderr bytes.Buffer
	code := runNew(newOpts{
		Slug:       "x",
		Type:       "banana",
		Repos:      []string{"api"},
		Base:       "main",
		NoTmux:     true,
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if code != 2 {
		t.Fatalf("want exit 2 (invalid_args), got %d", code)
	}
}

func TestNew_invalidSlug(t *testing.T) {
	root := t.TempDir()
	tasksDir := t.TempDir()
	cfgPath := writeConfigWithRoot(t, tasksDir, root)

	var stdout, stderr bytes.Buffer
	code := runNew(newOpts{
		Slug:       "NoUppercase",
		Type:       "feat",
		Repos:      []string{"api"},
		Base:       "main",
		NoTmux:     true,
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
}

func TestNew_taskExists(t *testing.T) {
	root := t.TempDir()
	api := testutil.InitFixtureRepo(t)
	if err := os.Rename(api, filepath.Join(root, "api")); err != nil {
		t.Fatal(err)
	}
	tasksDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tasksDir, "dup"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeConfigWithRoot(t, tasksDir, root)

	var stdout, stderr bytes.Buffer
	code := runNew(newOpts{
		Slug:       "dup",
		Type:       "feat",
		Repos:      []string{"api"},
		Base:       "main",
		NoTmux:     true,
		ConfigPath: cfgPath,
		Mode:       ModeJSON,
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if code != 4 {
		t.Fatalf("want exit 4 (task_exists), got %d", code)
	}
}
