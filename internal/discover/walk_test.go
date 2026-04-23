package discover

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// makeRepo creates a fake repo (dir with .git subdir) at dir/name.
func makeRepo(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Join(p, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWalk_emptyRoots(t *testing.T) {
	repos, err := Walk(nil)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(repos) != 0 {
		t.Fatalf("want empty, got %v", repos)
	}
}

func TestWalk_singleRootSingleRepo(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	repos, err := Walk([]string{root})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(repos) != 1 || repos[0].Name != "api" {
		t.Fatalf("want [api], got %+v", repos)
	}
	if repos[0].Path != filepath.Join(root, "api") {
		t.Fatalf("wrong path: %s", repos[0].Path)
	}
}

func TestWalk_skipsNonRepoDirs(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "api")
	if err := os.MkdirAll(filepath.Join(root, "not-a-repo/docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	repos, _ := Walk([]string{root})
	if len(repos) != 1 {
		t.Fatalf("want 1 repo, got %d (%+v)", len(repos), repos)
	}
}

func TestWalk_disambiguatesDuplicateNames(t *testing.T) {
	r1 := t.TempDir()
	r2 := t.TempDir()
	makeRepo(t, r1, "api")
	makeRepo(t, r2, "api")

	repos, err := Walk([]string{r1, r2})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("want 2 repos, got %d", len(repos))
	}

	names := []string{repos[0].Name, repos[1].Name}
	sort.Strings(names)
	// first appearance keeps plain name, second appearance gets <root-basename>/api
	// Order of Walk visits r1 first so repos[0].Name = "api" (plain).
	expected := []string{"api", filepath.Base(r2) + "/api"}
	sort.Strings(expected)
	if names[0] != expected[0] || names[1] != expected[1] {
		t.Fatalf("want %v, got %v", expected, names)
	}
}

func TestWalk_depth2Only(t *testing.T) {
	root := t.TempDir()
	// Depth 3 repo should NOT be discovered.
	deep := filepath.Join(root, "level1", "level2", "deep-repo")
	if err := os.MkdirAll(filepath.Join(deep, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Depth 2 repo SHOULD be discovered.
	makeRepo(t, root, "shallow")

	repos, _ := Walk([]string{root})
	if len(repos) != 1 || repos[0].Name != "shallow" {
		t.Fatalf("want [shallow], got %+v", repos)
	}
}
