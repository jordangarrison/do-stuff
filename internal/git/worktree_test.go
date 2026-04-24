package git_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jordangarrison/do-stuff/internal/errs"
	dsgit "github.com/jordangarrison/do-stuff/internal/git"
	"github.com/jordangarrison/do-stuff/internal/testutil"
)

func TestWorktreeAdd_createFromBase(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	if err := dsgit.WorktreeAdd(repo, wt, "feat/fresh", "main", dsgit.CreateFromBase); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, "README.md")); err != nil {
		t.Fatalf("worktree missing expected file: %v", err)
	}
	// branch must exist locally after CreateFromBase
	exists, err := dsgit.BranchExistsLocal(repo, "feat/fresh")
	if err != nil || !exists {
		t.Fatalf("expected local branch feat/fresh, err=%v exists=%v", err, exists)
	}
}

func TestWorktreeAdd_checkoutExisting(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	testutil.GitRun(t, repo, "branch", "feat/existing")

	wt := filepath.Join(t.TempDir(), "wt")
	if err := dsgit.WorktreeAdd(repo, wt, "feat/existing", "main", dsgit.CheckoutExisting); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wt, ".git")); err != nil {
		t.Fatalf("worktree .git missing: %v", err)
	}
}

func TestWorktreeAdd_fetchAndTrack(t *testing.T) {
	work, remote := testutil.InitFixtureRepoWithRemote(t)

	// Create a feature branch on the remote by pushing from a second clone.
	pusher := t.TempDir()
	testutil.GitRun(t, pusher, "clone", "-q", remote, ".")
	testutil.GitRun(t, pusher, "config", "user.email", "t@x")
	testutil.GitRun(t, pusher, "config", "user.name", "t")
	testutil.GitRun(t, pusher, "config", "commit.gpgsign", "false")
	testutil.GitRun(t, pusher, "checkout", "-q", "-b", "feat/remote-only")
	if err := os.WriteFile(filepath.Join(pusher, "f"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.GitRun(t, pusher, "add", "f")
	testutil.GitRun(t, pusher, "commit", "-q", "-m", "add")
	testutil.GitRun(t, pusher, "push", "-q", "origin", "feat/remote-only")

	// From the original work clone the branch should not exist locally yet.
	hasLocal, err := dsgit.BranchExistsLocal(work, "feat/remote-only")
	if err != nil || hasLocal {
		t.Fatalf("precondition: local branch should be absent, err=%v has=%v", err, hasLocal)
	}

	wt := filepath.Join(t.TempDir(), "wt")
	if err := dsgit.WorktreeAdd(work, wt, "feat/remote-only", "main", dsgit.FetchAndTrack); err != nil {
		t.Fatalf("WorktreeAdd FetchAndTrack: %v", err)
	}
	hasLocal, err = dsgit.BranchExistsLocal(work, "feat/remote-only")
	if err != nil || !hasLocal {
		t.Fatalf("expected local branch after FetchAndTrack, err=%v has=%v", err, hasLocal)
	}
}

func TestWorktreeDirty_cleanFalse(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	if err := dsgit.WorktreeAdd(repo, wt, "feat/clean", "main", dsgit.CreateFromBase); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	dirty, err := dsgit.WorktreeDirty(wt)
	if err != nil {
		t.Fatalf("WorktreeDirty: %v", err)
	}
	if dirty {
		t.Fatal("expected clean worktree, got dirty")
	}
}

func TestWorktreeDirty_dirtyTrue(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	if err := dsgit.WorktreeAdd(repo, wt, "feat/dirty", "main", dsgit.CreateFromBase); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wt, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err := dsgit.WorktreeDirty(wt)
	if err != nil {
		t.Fatalf("WorktreeDirty: %v", err)
	}
	if !dirty {
		t.Fatal("expected dirty worktree, got clean")
	}
}

func TestWorktreeRemove_clean(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	if err := dsgit.WorktreeAdd(repo, wt, "feat/rm-clean", "main", dsgit.CreateFromBase); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if err := dsgit.WorktreeRemove(repo, wt, false); err != nil {
		t.Fatalf("WorktreeRemove: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree dir still present: %v", err)
	}
}

func TestWorktreeRemove_dirtyUnforced(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	if err := dsgit.WorktreeAdd(repo, wt, "feat/rm-dirty", "main", dsgit.CreateFromBase); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wt, "dirt.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := dsgit.WorktreeRemove(repo, wt, false)
	if err == nil {
		t.Fatal("expected error removing dirty worktree without force")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) || te.Code != errs.GitError {
		t.Fatalf("want GitError, got %v", err)
	}
}

func TestWorktreeRemove_dirtyForced(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	if err := dsgit.WorktreeAdd(repo, wt, "feat/rm-force", "main", dsgit.CreateFromBase); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wt, "dirt.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := dsgit.WorktreeRemove(repo, wt, true); err != nil {
		t.Fatalf("WorktreeRemove force: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree dir still present: %v", err)
	}
}

func TestWorktreeRemove_missingAndUnregistered(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	// Path that was never a worktree of this repo and doesn't exist.
	ghost := filepath.Join(t.TempDir(), "ghost")
	if err := dsgit.WorktreeRemove(repo, ghost, true); err != nil {
		t.Fatalf("WorktreeRemove on missing+unregistered path must be nil, got %v", err)
	}
}

func TestWorktreeAdd_alreadyExistsMapsToWorktreeExists(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)

	// Pre-create the target path so `git worktree add` fails with
	// "already exists". Using a non-empty directory is the simplest
	// way to trigger this; an empty dir also works.
	wt := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := dsgit.WorktreeAdd(repo, wt, "feat/collide", "main", dsgit.CreateFromBase)
	if err == nil {
		t.Fatal("expected worktree add to fail with path collision")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) {
		t.Fatalf("want TaskError, got %T", err)
	}
	if te.Code != errs.WorktreeExists {
		t.Fatalf("want code %q, got %q (stderr=%v)", errs.WorktreeExists, te.Code, te.Details["git_stderr"])
	}
}
