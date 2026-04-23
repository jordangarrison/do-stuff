// Package testutil provides test-only helpers shared across packages.
// Kept outside *_test.go so it can be imported from any test file in the
// module; the public Go build never pulls it in because nothing non-test
// imports it.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// InitFixtureRepo builds a fresh git repo at t.TempDir(), sets a throwaway
// identity so commits don't depend on user git config, and lands one commit
// on branch `main`. Returns the repo's absolute path.
func InitFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "test")
	gitRun(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-q", "-m", "init")
	return dir
}

// InitFixtureRepoWithRemote builds a bare remote repo and a working clone
// whose origin points at it. main is pushed so remote-branch lookups work.
// Returns (workTreePath, remotePath).
func InitFixtureRepoWithRemote(t *testing.T) (string, string) {
	t.Helper()
	remote := t.TempDir()
	gitRun(t, remote, "init", "-q", "--bare", "-b", "main")
	work := InitFixtureRepo(t)
	gitRun(t, work, "remote", "add", "origin", remote)
	gitRun(t, work, "push", "-q", "origin", "main")
	return work, remote
}

// GitRun invokes git against dir with the given args and fails the test on
// any non-zero exit. Exported so test files can shell git directly when
// they need operations beyond InitFixtureRepo's scope (branch creation,
// remote pushes, etc).
func GitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	gitRun(t, dir, args...)
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v (in %s): %v\n%s", args, dir, err, out)
	}
}
