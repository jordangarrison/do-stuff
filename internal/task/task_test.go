package task_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jordangarrison/do-stuff/internal/task"
)

func TestWriteLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 4, 23, 15, 4, 5, 0, time.UTC)
	src := &task.Task{
		Slug:        "infra-6700-auth-refactor",
		Type:        "feat",
		Ticket:      "INFRA-6700",
		TicketURL:   "https://flocasts.atlassian.net/browse/INFRA-6700",
		Branch:      "feat/infra-6700-auth-refactor",
		Base:        "main",
		TmuxSession: "task-infra-6700-auth-refactor",
		CreatedAt:   now,
		Repos: []task.RepoRef{
			{Name: "api", Path: "/abs/api", Worktree: "api"},
			{Name: "web", Path: "/abs/web", Worktree: "web"},
		},
	}
	if err := task.Write(dir, src); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := task.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Slug != src.Slug || got.Branch != src.Branch {
		t.Fatalf("round trip lost fields: %+v", got)
	}
	if len(got.Repos) != 2 || got.Repos[1].Worktree != "web" {
		t.Fatalf("bad repos: %+v", got.Repos)
	}
	if !got.CreatedAt.Equal(now) {
		t.Fatalf("created_at: want %v got %v", now, got.CreatedAt)
	}
}

func TestLoad_missingFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := task.Load(dir); err == nil {
		t.Fatal("expected error for missing .task.json")
	}
}

func TestWrite_filePath(t *testing.T) {
	dir := t.TempDir()
	if err := task.Write(dir, &task.Task{Slug: "x", Branch: "feat/x", Base: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := filepath.Abs(filepath.Join(dir, ".task.json")); err != nil {
		t.Fatal(err)
	}
}
