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

func TestBranchExistsLocal(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	has, err := dsgit.BranchExistsLocal(repo, "main")
	if err != nil || !has {
		t.Fatalf("main missing? err=%v has=%v", err, has)
	}
	has, err = dsgit.BranchExistsLocal(repo, "nope")
	if err != nil || has {
		t.Fatalf("nope present? err=%v has=%v", err, has)
	}
}

func TestBranchExistsRemote(t *testing.T) {
	work, _ := testutil.InitFixtureRepoWithRemote(t)
	has, err := dsgit.BranchExistsRemote(work, "origin", "main")
	if err != nil || !has {
		t.Fatalf("origin/main missing? err=%v has=%v", err, has)
	}
	has, err = dsgit.BranchExistsRemote(work, "origin", "nope")
	if err != nil || has {
		t.Fatalf("origin/nope present? err=%v has=%v", err, has)
	}
}

func TestFetchBranch(t *testing.T) {
	work, remote := testutil.InitFixtureRepoWithRemote(t)

	// Push a new branch from a second clone.
	pusher := t.TempDir()
	testutil.GitRun(t, pusher, "clone", "-q", remote, ".")
	testutil.GitRun(t, pusher, "config", "user.email", "t@x")
	testutil.GitRun(t, pusher, "config", "user.name", "t")
	testutil.GitRun(t, pusher, "config", "commit.gpgsign", "false")
	testutil.GitRun(t, pusher, "checkout", "-q", "-b", "feat/x")
	testutil.GitRun(t, pusher, "commit", "-q", "--allow-empty", "-m", "x")
	testutil.GitRun(t, pusher, "push", "-q", "origin", "feat/x")

	if err := dsgit.FetchBranch(work, "origin", "feat/x"); err != nil {
		t.Fatalf("FetchBranch: %v", err)
	}
	has, err := dsgit.BranchExistsRemote(work, "origin", "feat/x")
	if err != nil || !has {
		t.Fatalf("post-fetch remote branch missing, err=%v has=%v", err, has)
	}
}

func TestBranchDelete_merged(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	testutil.GitRun(t, repo, "branch", "feat/merged")
	if err := dsgit.BranchDelete(repo, "feat/merged", false); err != nil {
		t.Fatalf("BranchDelete: %v", err)
	}
	has, err := dsgit.BranchExistsLocal(repo, "feat/merged")
	if err != nil || has {
		t.Fatalf("branch still present, err=%v has=%v", err, has)
	}
}

func TestBranchDelete_unmergedUnforced(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	testutil.GitRun(t, repo, "checkout", "-q", "-b", "feat/unmerged")
	if err := os.WriteFile(filepath.Join(repo, "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.GitRun(t, repo, "add", "x.txt")
	testutil.GitRun(t, repo, "commit", "-q", "-m", "x")
	testutil.GitRun(t, repo, "checkout", "-q", "main")

	err := dsgit.BranchDelete(repo, "feat/unmerged", false)
	if err == nil {
		t.Fatal("expected error deleting unmerged branch without force")
	}
	var te *errs.TaskError
	if !errors.As(err, &te) || te.Code != errs.GitError {
		t.Fatalf("want GitError, got %v", err)
	}
}

func TestBranchDelete_unmergedForced(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	testutil.GitRun(t, repo, "checkout", "-q", "-b", "feat/unmerged-force")
	if err := os.WriteFile(filepath.Join(repo, "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.GitRun(t, repo, "add", "x.txt")
	testutil.GitRun(t, repo, "commit", "-q", "-m", "x")
	testutil.GitRun(t, repo, "checkout", "-q", "main")

	if err := dsgit.BranchDelete(repo, "feat/unmerged-force", true); err != nil {
		t.Fatalf("BranchDelete force: %v", err)
	}
	has, err := dsgit.BranchExistsLocal(repo, "feat/unmerged-force")
	if err != nil || has {
		t.Fatalf("branch still present, err=%v has=%v", err, has)
	}
}

func TestBranchDelete_missingBranchSwallowed(t *testing.T) {
	repo := testutil.InitFixtureRepo(t)
	if err := dsgit.BranchDelete(repo, "feat/never-was", false); err != nil {
		t.Fatalf("expected missing-branch to be swallowed, got %v", err)
	}
}
