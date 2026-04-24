package task_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/task"
	"github.com/jordangarrison/do-stuff/internal/tmux"
)

func writeTaskJSON(t *testing.T, dir string, tk *task.Task) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := task.Write(dir, tk); err != nil {
		t.Fatal(err)
	}
}

func TestAttach_taskNotFound(t *testing.T) {
	tasksDir := t.TempDir()
	_, err := task.Attach(task.AttachParams{
		Slug:       "does-not-exist",
		TasksDir:   tasksDir,
		TmuxPrefix: "task-",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) {
		t.Fatalf("want TaskError, got %T", err)
	}
	if te.Code != errs.TaskNotFound {
		t.Fatalf("want TaskNotFound, got %s", te.Code)
	}
}

func TestAttach_metadataHasNoSessionAndNoStartTmux(t *testing.T) {
	tasksDir := t.TempDir()
	slug := "nosession"
	writeTaskJSON(t, filepath.Join(tasksDir, slug), &task.Task{
		Slug:      slug,
		Type:      "feat",
		Branch:    "feat/nosession",
		Base:      "main",
		Repos:     []task.RepoRef{{Name: "api", Path: "/tmp/api", Worktree: "api"}},
		CreatedAt: time.Now().UTC(),
	})

	_, err := task.Attach(task.AttachParams{
		Slug:       slug,
		TasksDir:   tasksDir,
		TmuxPrefix: "task-",
	})
	if err == nil {
		t.Fatal("expected TmuxSessionMissing")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) {
		t.Fatalf("want TaskError, got %T", err)
	}
	if te.Code != errs.TmuxSessionMissing {
		t.Fatalf("want TmuxSessionMissing, got %s", te.Code)
	}
}

// requireTmux skips when tmux isn't on PATH; isolates per test via TMUX_TMPDIR.
func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	t.Setenv("TMUX_TMPDIR", t.TempDir())
}

// seedTaskWithWorktreeDirs writes .task.json and creates placeholder worktree
// dirs so tmux `-c <cwd>` succeeds. Returns the task dir.
func seedTaskWithWorktreeDirs(t *testing.T, tasksDir, slug, session string, repos []string) string {
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
		Branch:      "feat/" + slug,
		Base:        "main",
		Repos:       refs,
		TmuxSession: session,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	return taskDir
}

func TestAttach_sessionAliveReturnsWasRecreatedFalse(t *testing.T) {
	requireTmux(t)
	tasksDir := t.TempDir()
	slug := "alive"
	session := "ds-test-" + slug
	taskDir := seedTaskWithWorktreeDirs(t, tasksDir, slug, session, []string{"api"})
	t.Cleanup(func() { _ = tmux.KillSession(session) })
	if err := tmux.NewSession(session, "api", filepath.Join(taskDir, "api")); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	res, err := task.Attach(task.AttachParams{
		Slug: slug, TasksDir: tasksDir, TmuxPrefix: "ds-test-",
	})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if res.WasRecreated {
		t.Fatalf("expected WasRecreated=false")
	}
	if res.SessionName != session {
		t.Fatalf("want %q, got %q", session, res.SessionName)
	}
}

func TestAttach_sessionDeadRecreates(t *testing.T) {
	requireTmux(t)
	tasksDir := t.TempDir()
	slug := "dead"
	session := "ds-test-" + slug
	_ = seedTaskWithWorktreeDirs(t, tasksDir, slug, session, []string{"api", "web"})
	t.Cleanup(func() { _ = tmux.KillSession(session) })
	// No seed session created -> Attach must recreate.

	res, err := task.Attach(task.AttachParams{
		Slug: slug, TasksDir: tasksDir, TmuxPrefix: "ds-test-",
	})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if !res.WasRecreated {
		t.Fatal("expected WasRecreated=true")
	}
	has, err := tmux.HasSession(session)
	if err != nil || !has {
		t.Fatalf("expected session present, has=%v err=%v", has, err)
	}
}

func TestAttach_worktreeMissing(t *testing.T) {
	requireTmux(t)
	tasksDir := t.TempDir()
	slug := "broken"
	session := "ds-test-" + slug
	taskDir := seedTaskWithWorktreeDirs(t, tasksDir, slug, session, []string{"api", "web"})
	// Remove one worktree dir AFTER seeding so metadata still references it.
	if err := os.RemoveAll(filepath.Join(taskDir, "web")); err != nil {
		t.Fatal(err)
	}

	_, err := task.Attach(task.AttachParams{
		Slug: slug, TasksDir: tasksDir, TmuxPrefix: "ds-test-",
	})
	if err == nil {
		t.Fatal("expected WorktreeMissing")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) {
		t.Fatalf("want TaskError, got %T", err)
	}
	if te.Code != errs.WorktreeMissing {
		t.Fatalf("want WorktreeMissing, got %s", te.Code)
	}
	// Assert tmux was NOT touched by checking the session isn't there.
	has, _ := tmux.HasSession(session)
	if has {
		t.Fatalf("tmux session should not have been created before preflight failed")
	}
}

func TestAttach_startTmuxFabricatesAndPersists(t *testing.T) {
	requireTmux(t)
	tasksDir := t.TempDir()
	slug := "fresh"
	_ = seedTaskWithWorktreeDirs(t, tasksDir, slug, "", []string{"api"})
	expected := "ds-test-" + slug
	t.Cleanup(func() { _ = tmux.KillSession(expected) })

	res, err := task.Attach(task.AttachParams{
		Slug: slug, TasksDir: tasksDir, TmuxPrefix: "ds-test-", StartTmux: true,
	})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if res.SessionName != expected {
		t.Fatalf("want %q, got %q", expected, res.SessionName)
	}
	if !res.WasRecreated {
		t.Fatal("expected WasRecreated=true")
	}
	// Metadata must have been updated.
	reloaded, err := task.Load(filepath.Join(tasksDir, slug))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.TmuxSession != expected {
		t.Fatalf("want persisted session %q, got %q", expected, reloaded.TmuxSession)
	}
}

func TestAttach_startTmuxPersistsWhenSessionAlreadyAlive(t *testing.T) {
	requireTmux(t)
	tasksDir := t.TempDir()
	slug := "existing"
	expected := "ds-test-" + slug
	taskDir := seedTaskWithWorktreeDirs(t, tasksDir, slug, "", []string{"api"})
	t.Cleanup(func() { _ = tmux.KillSession(expected) })

	// Pre-create the session that --start-tmux would fabricate.
	if err := tmux.NewSession(expected, "api", filepath.Join(taskDir, "api")); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	res, err := task.Attach(task.AttachParams{
		Slug: slug, TasksDir: tasksDir, TmuxPrefix: "ds-test-", StartTmux: true,
	})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if res.WasRecreated {
		t.Fatal("expected WasRecreated=false (session already alive)")
	}
	if res.SessionName != expected {
		t.Fatalf("want session %q, got %q", expected, res.SessionName)
	}
	// Metadata must have been persisted so future plain `ds attach`
	// doesn't re-trigger tmux_session_not_found.
	reloaded, err := task.Load(filepath.Join(tasksDir, slug))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.TmuxSession != expected {
		t.Fatalf("want persisted session %q, got %q", expected, reloaded.TmuxSession)
	}
}

func TestAttach_emptyReposInMetadata(t *testing.T) {
	tasksDir := t.TempDir()
	slug := "empty"
	writeTaskJSON(t, filepath.Join(tasksDir, slug), &task.Task{
		Slug:      slug,
		Type:      "feat",
		Branch:    "feat/" + slug,
		Base:      "main",
		Repos:     nil, // corrupted metadata
		CreatedAt: time.Now().UTC(),
	})

	_, err := task.Attach(task.AttachParams{
		Slug: slug, TasksDir: tasksDir, TmuxPrefix: "ds-test-",
	})
	if err == nil {
		t.Fatal("expected Internal error, got nil (likely panicked)")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) {
		t.Fatalf("want TaskError, got %T", err)
	}
	if te.Code != errs.Internal {
		t.Fatalf("want Internal, got %s", te.Code)
	}
}
