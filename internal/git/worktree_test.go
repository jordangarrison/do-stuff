package git_test

import (
	"os"
	"path/filepath"
	"testing"

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
