package git_test

import (
	"testing"

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
