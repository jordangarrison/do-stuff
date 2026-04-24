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
	"github.com/jordangarrison/do-stuff/internal/testutil"
)

// finishSeed wires up a tasks dir plus a matching per-repo fixture git
// repo whose branch `feat/<slug>` is already created and worktree-added
// at `<tasksDir>/<slug>/<repo>`. Returns the task dir path.
func finishSeed(t *testing.T, tasksDir, slug string, repos []string, session string) (string, []string) {
	t.Helper()
	taskDir := filepath.Join(tasksDir, slug)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}

	branch := "feat/" + slug
	refs := make([]task.RepoRef, 0, len(repos))
	srcPaths := make([]string, 0, len(repos))
	for _, name := range repos {
		src := testutil.InitFixtureRepo(t)
		srcPaths = append(srcPaths, src)
		wt := filepath.Join(taskDir, name)
		testutil.GitRun(t, src, "worktree", "add", "-b", branch, wt, "main")
		// Stash the per-repo branch so we can assert deletion later.
		refs = append(refs, task.RepoRef{Name: name, Path: src, Worktree: name})
	}
	tk := &task.Task{
		Slug:        slug,
		Type:        "feat",
		Branch:      branch,
		Base:        "main",
		Repos:       refs,
		TmuxSession: session,
		CreatedAt:   time.Now().UTC(),
	}
	if err := task.Write(taskDir, tk); err != nil {
		t.Fatal(err)
	}
	return taskDir, srcPaths
}

func TestFinish_taskNotFound(t *testing.T) {
	tasksDir := t.TempDir()
	_, err := task.Finish(task.FinishParams{
		Slug:     "does-not-exist",
		TasksDir: tasksDir,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) || te.Code != errs.TaskNotFound {
		t.Fatalf("want TaskNotFound, got %v", err)
	}
}

func TestFinish_happyPath(t *testing.T) {
	tasksDir := t.TempDir()
	taskDir, srcPaths := finishSeed(t, tasksDir, "happy", []string{"api", "web"}, "")

	res, err := task.Finish(task.FinishParams{
		Slug:     "happy",
		TasksDir: tasksDir,
	})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got, want := res.RemovedWorktrees, []string{"api", "web"}; !equalStrings(got, want) {
		t.Fatalf("RemovedWorktrees=%v want %v", got, want)
	}
	if res.KilledSession != "" {
		t.Fatalf("KilledSession=%q want empty", res.KilledSession)
	}
	if res.BranchesKept {
		t.Fatal("BranchesKept=true; want false")
	}
	if _, err := os.Stat(taskDir); !os.IsNotExist(err) {
		t.Fatalf("task dir still present: %v", err)
	}
	// Branches deleted in each source repo.
	for _, src := range srcPaths {
		cmd := exec.Command("git", "-C", src, "rev-parse", "--verify", "--quiet", "refs/heads/feat/happy")
		if err := cmd.Run(); err == nil {
			t.Fatalf("branch feat/happy still in %s", src)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFinish_dirtyWithoutForce(t *testing.T) {
	tasksDir := t.TempDir()
	taskDir, _ := finishSeed(t, tasksDir, "dirty", []string{"api"}, "")
	if err := os.WriteFile(filepath.Join(taskDir, "api", "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := task.Finish(task.FinishParams{
		Slug:     "dirty",
		TasksDir: tasksDir,
	})
	if err == nil {
		t.Fatal("expected WorktreeDirty")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) || te.Code != errs.WorktreeDirty {
		t.Fatalf("want WorktreeDirty, got %v", err)
	}
	if _, err := os.Stat(taskDir); err != nil {
		t.Fatalf("task dir must still exist on dirty-guard error: %v", err)
	}
}

func TestFinish_dirtyWithForce(t *testing.T) {
	tasksDir := t.TempDir()
	taskDir, _ := finishSeed(t, tasksDir, "force", []string{"api"}, "")
	if err := os.WriteFile(filepath.Join(taskDir, "api", "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := task.Finish(task.FinishParams{
		Slug:     "force",
		TasksDir: tasksDir,
		Force:    true,
	})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got, want := res.RemovedWorktrees, []string{"api"}; !equalStrings(got, want) {
		t.Fatalf("RemovedWorktrees=%v want %v", got, want)
	}
	if _, err := os.Stat(taskDir); !os.IsNotExist(err) {
		t.Fatalf("task dir still present: %v", err)
	}
}

func TestFinish_missingWorktreeDirSkipped(t *testing.T) {
	tasksDir := t.TempDir()
	taskDir, _ := finishSeed(t, tasksDir, "partial", []string{"api", "web"}, "")
	if err := os.RemoveAll(filepath.Join(taskDir, "web")); err != nil {
		t.Fatal(err)
	}

	res, err := task.Finish(task.FinishParams{
		Slug:     "partial",
		TasksDir: tasksDir,
	})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got, want := res.RemovedWorktrees, []string{"api", "web"}; !equalStrings(got, want) {
		t.Fatalf("RemovedWorktrees=%v want %v", got, want)
	}
}

func TestFinish_keepBranches(t *testing.T) {
	tasksDir := t.TempDir()
	_, srcPaths := finishSeed(t, tasksDir, "keep", []string{"api"}, "")

	res, err := task.Finish(task.FinishParams{
		Slug:         "keep",
		TasksDir:     tasksDir,
		KeepBranches: true,
	})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if !res.BranchesKept {
		t.Fatal("BranchesKept=false; want true")
	}
	cmd := exec.Command("git", "-C", srcPaths[0], "rev-parse", "--verify", "--quiet", "refs/heads/feat/keep")
	if err := cmd.Run(); err != nil {
		t.Fatalf("branch feat/keep missing in %s: %v", srcPaths[0], err)
	}
}

func TestFinish_idempotentReRun(t *testing.T) {
	tasksDir := t.TempDir()
	finishSeed(t, tasksDir, "again", []string{"api"}, "")

	if _, err := task.Finish(task.FinishParams{Slug: "again", TasksDir: tasksDir}); err != nil {
		t.Fatalf("first Finish: %v", err)
	}
	_, err := task.Finish(task.FinishParams{Slug: "again", TasksDir: tasksDir})
	if err == nil {
		t.Fatal("expected TaskNotFound on second call")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) || te.Code != errs.TaskNotFound {
		t.Fatalf("want TaskNotFound, got %v", err)
	}
}

func TestFinish_killsTmuxSession(t *testing.T) {
	requireTmux(t)
	tasksDir := t.TempDir()
	session := "ds-test-finish-kill"
	taskDir, _ := finishSeed(t, tasksDir, "tmux-kill", []string{"api"}, session)

	if err := exec.Command("tmux", "new-session", "-d", "-s", session, "-c", filepath.Join(taskDir, "api")).Run(); err != nil {
		t.Fatalf("tmux new-session: %v", err)
	}

	res, err := task.Finish(task.FinishParams{Slug: "tmux-kill", TasksDir: tasksDir})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if res.KilledSession != session {
		t.Fatalf("KilledSession=%q want %q", res.KilledSession, session)
	}
	if err := exec.Command("tmux", "has-session", "-t", "="+session).Run(); err == nil {
		t.Fatal("tmux session still exists")
	}
}

func TestFinish_partialFailureRerun(t *testing.T) {
	tasksDir := t.TempDir()
	taskDir, srcPaths := finishSeed(t, tasksDir, "partial-fail", []string{"api"}, "")
	// Simulate the state left by a prior ds finish that completed WorktreeRemove
	// but then failed later (e.g. tmux kill): both the worktree dir AND its git
	// registration are gone, but .task.json remains.
	if err := exec.Command("git", "-C", srcPaths[0], "worktree", "remove", "--force", filepath.Join(taskDir, "api")).Run(); err != nil {
		t.Fatalf("seed worktree remove: %v", err)
	}

	res, err := task.Finish(task.FinishParams{Slug: "partial-fail", TasksDir: tasksDir})
	if err != nil {
		t.Fatalf("Finish rerun must succeed after partial failure, got %v", err)
	}
	if got, want := res.RemovedWorktrees, []string{"api"}; !equalStrings(got, want) {
		t.Fatalf("RemovedWorktrees=%v want %v", got, want)
	}
	if _, err := os.Stat(taskDir); !os.IsNotExist(err) {
		t.Fatalf("task dir still present: %v", err)
	}
}

func TestFinish_sessionAlreadyGone(t *testing.T) {
	requireTmux(t)
	tasksDir := t.TempDir()
	finishSeed(t, tasksDir, "no-session", []string{"api"}, "ds-test-finish-absent")

	res, err := task.Finish(task.FinishParams{Slug: "no-session", TasksDir: tasksDir})
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if res.KilledSession != "" {
		t.Fatalf("KilledSession=%q want empty", res.KilledSession)
	}
}
