package task_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/task"
	"github.com/jordangarrison/do-stuff/internal/testutil"
	"github.com/jordangarrison/do-stuff/internal/tmux"
)

func TestCreate_createsWorktreesAndMetadata_noTmux(t *testing.T) {
	apiRepo := testutil.InitFixtureRepo(t)
	webRepo := testutil.InitFixtureRepo(t)
	tasksDir := t.TempDir()

	res, err := task.Create(task.CreateParams{
		Slug:       "demo",
		Type:       "feat",
		Base:       "main",
		TasksDir:   tasksDir,
		Repos:      []task.ResolvedRepo{{Name: "api", Path: apiRepo}, {Name: "web", Path: webRepo}},
		NoTmux:     true,
		StartTmux:  true,
		TmuxPrefix: "task-",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Task.Branch != "feat/demo" {
		t.Fatalf("branch: %q", res.Task.Branch)
	}
	if res.Task.TmuxSession != "" {
		t.Fatalf("NoTmux: TmuxSession should be empty, got %q", res.Task.TmuxSession)
	}
	for _, r := range res.Task.Repos {
		if _, err := os.Stat(filepath.Join(tasksDir, "demo", r.Worktree, ".git")); err != nil {
			t.Fatalf("worktree for %s missing: %v", r.Name, err)
		}
	}
	// .task.json must be on disk
	loaded, err := task.Load(filepath.Join(tasksDir, "demo"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Branch != "feat/demo" {
		t.Fatalf("loaded branch: %q", loaded.Branch)
	}
	// branch states recorded
	if len(res.RepoStates) != 2 || res.RepoStates[0].BranchState != "created" {
		t.Fatalf("bad RepoStates: %+v", res.RepoStates)
	}
}

func TestCreate_branchTemplateWithTicket(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	tasksDir := t.TempDir()
	res, err := task.Create(task.CreateParams{
		Slug:      "auth-refactor",
		Type:      "feat",
		Ticket:    "INFRA-6700",
		Base:      "main",
		TasksDir:  tasksDir,
		Repos:     []task.ResolvedRepo{{Name: "api", Path: repo}},
		NoTmux:    true,
		StartTmux: false,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Task.Branch != "feat/infra-6700-auth-refactor" {
		t.Fatalf("branch: %q", res.Task.Branch)
	}
}

func TestCreate_branchOverride(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	tasksDir := t.TempDir()
	res, err := task.Create(task.CreateParams{
		Slug:           "whatever",
		Type:           "feat",
		BranchOverride: "hotfix/xyz",
		Base:           "main",
		TasksDir:       tasksDir,
		Repos:          []task.ResolvedRepo{{Name: "api", Path: repo}},
		NoTmux:         true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Task.Branch != "hotfix/xyz" {
		t.Fatalf("branch: %q", res.Task.Branch)
	}
}

func TestCreate_taskExists(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	tasksDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tasksDir, "dup"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := task.Create(task.CreateParams{
		Slug:     "dup",
		Type:     "feat",
		Base:     "main",
		TasksDir: tasksDir,
		Repos:    []task.ResolvedRepo{{Name: "api", Path: repo}},
		NoTmux:   true,
	})
	if err == nil {
		t.Fatal("expected task_exists error")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) || te.Code != errs.TaskExists {
		t.Fatalf("want task_exists, got %+v", err)
	}
}

func TestCreate_checkoutExisting(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	testutil.GitRun(t, repo, "branch", "feat/preexisting")

	tasksDir := t.TempDir()
	res, err := task.Create(task.CreateParams{
		Slug:     "preexisting",
		Type:     "feat",
		Base:     "main",
		TasksDir: tasksDir,
		Repos:    []task.ResolvedRepo{{Name: "api", Path: repo}},
		NoTmux:   true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.RepoStates[0].BranchState != "checked_out_existing" {
		t.Fatalf("state: %q", res.RepoStates[0].BranchState)
	}
}

func TestCreate_strictRejectsExistingBranch(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	testutil.GitRun(t, repo, "branch", "feat/strict")
	tasksDir := t.TempDir()

	_, err := task.Create(task.CreateParams{
		Slug:     "strict",
		Type:     "feat",
		Base:     "main",
		TasksDir: tasksDir,
		Repos:    []task.ResolvedRepo{{Name: "api", Path: repo}},
		NoTmux:   true,
		Strict:   true,
	})
	if err == nil {
		t.Fatal("expected branch_conflict")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) || te.Code != errs.BranchConflict {
		t.Fatalf("want branch_conflict, got %+v", err)
	}
}

func TestCreate_fetchedTracking(t *testing.T) {
	work, remote := testutil.InitFixtureRepoWithRemote(t)
	// Create remote-only branch.
	pusher := t.TempDir()
	testutil.GitRun(t, pusher, "clone", "-q", remote, ".")
	testutil.GitRun(t, pusher, "config", "user.email", "t@x")
	testutil.GitRun(t, pusher, "config", "user.name", "t")
	testutil.GitRun(t, pusher, "config", "commit.gpgsign", "false")
	testutil.GitRun(t, pusher, "checkout", "-q", "-b", "feat/remote")
	testutil.GitRun(t, pusher, "commit", "-q", "--allow-empty", "-m", "x")
	testutil.GitRun(t, pusher, "push", "-q", "origin", "feat/remote")

	tasksDir := t.TempDir()
	res, err := task.Create(task.CreateParams{
		Slug:           "rt",
		Type:           "feat",
		BranchOverride: "feat/remote",
		Base:           "main",
		TasksDir:       tasksDir,
		Repos:          []task.ResolvedRepo{{Name: "api", Path: work}},
		NoTmux:         true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.RepoStates[0].BranchState != "fetched_tracking" {
		t.Fatalf("state: %q", res.RepoStates[0].BranchState)
	}
}

func TestCreate_withTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	t.Setenv("TMUX_TMPDIR", t.TempDir())

	repo := testutil.InitFixtureRepo(t)
	tasksDir := t.TempDir()
	res, err := task.Create(task.CreateParams{
		Slug:       "withtmux",
		Type:       "feat",
		Base:       "main",
		TasksDir:   tasksDir,
		Repos:      []task.ResolvedRepo{{Name: "api", Path: repo}},
		StartTmux:  true,
		TmuxPrefix: "task-",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Task.TmuxSession != "task-withtmux" {
		t.Fatalf("session: %q", res.Task.TmuxSession)
	}
}

func TestCreate_strictBranchConflictLeavesNoTaskDir(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	testutil.GitRun(t, repo, "branch", "feat/conflict")
	tasksDir := t.TempDir()

	_, err := task.Create(task.CreateParams{
		Slug:     "conflict",
		Type:     "feat",
		Base:     "main",
		TasksDir: tasksDir,
		Repos:    []task.ResolvedRepo{{Name: "api", Path: repo}},
		NoTmux:   true,
		Strict:   true,
	})
	var te *errs.TaskError
	if !errors.As(err, &te) || te.Code != errs.BranchConflict {
		t.Fatalf("want branch_conflict, got %+v", err)
	}
	// The task dir must not be left behind.
	if _, statErr := os.Stat(filepath.Join(tasksDir, "conflict")); !os.IsNotExist(statErr) {
		t.Fatalf("task dir leaked after branch_conflict preflight; stat err=%v", statErr)
	}
}

func TestCreate_tmuxSessionExistsLeavesNoTaskDir(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not on PATH")
	}
	t.Setenv("TMUX_TMPDIR", t.TempDir())

	repo := testutil.InitFixtureRepo(t)
	tasksDir := t.TempDir()
	sessionName := "task-preflight"

	// Pre-create the session so the tmux preflight fires.
	if err := tmux.NewSession(sessionName, "x", t.TempDir()); err != nil {
		t.Fatalf("NewSession preflight prep: %v", err)
	}
	t.Cleanup(func() { _ = tmux.KillSession(sessionName) })

	_, err := task.Create(task.CreateParams{
		Slug:       "preflight",
		Type:       "feat",
		Base:       "main",
		TasksDir:   tasksDir,
		Repos:      []task.ResolvedRepo{{Name: "api", Path: repo}},
		StartTmux:  true,
		TmuxPrefix: "task-",
	})
	var te *errs.TaskError
	if !errors.As(err, &te) || te.Code != errs.TmuxSessionExists {
		t.Fatalf("want tmux_session_exists, got %+v", err)
	}
	// No task dir, no worktrees on disk.
	if _, statErr := os.Stat(filepath.Join(tasksDir, "preflight")); !os.IsNotExist(statErr) {
		t.Fatalf("task dir leaked after tmux preflight; stat err=%v", statErr)
	}
}
