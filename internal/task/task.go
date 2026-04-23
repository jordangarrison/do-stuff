// Package task defines the Task metadata and the Create orchestrator used
// by `ds new`. The package owns .task.json round-trip and composes
// internal/git + internal/tmux to realize a task on disk.
package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

// MetadataFile is the on-disk filename that holds a task's metadata
// inside <tasks_dir>/<slug>/.
const MetadataFile = ".task.json"

// Task is the persisted description of one task. JSON tags mirror the
// envelope SPEC exactly so Load/Write also define the public schema.
type Task struct {
	Slug        string    `json:"slug"`
	Type        string    `json:"type"`
	Ticket      string    `json:"ticket,omitempty"`
	TicketURL   string    `json:"ticket_url,omitempty"`
	Branch      string    `json:"branch"`
	Base        string    `json:"base"`
	Repos       []RepoRef `json:"repos"`
	TmuxSession string    `json:"tmux_session,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// RepoRef is one entry in Task.Repos. Worktree is a directory name
// relative to <tasks_dir>/<slug>/; Path is the absolute source repo.
type RepoRef struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Worktree string `json:"worktree"`
}

// Load reads <taskDir>/.task.json into a Task.
func Load(taskDir string) (*Task, error) {
	p := filepath.Join(taskDir, MetadataFile)
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, &errs.TaskError{
			Code:    errs.TaskNotFound,
			Message: fmt.Sprintf("reading %s: %v", p, err),
			Details: map[string]any{"path": p},
		}
	}
	var t Task
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, &errs.TaskError{
			Code:    errs.Internal,
			Message: fmt.Sprintf("decoding %s: %v", p, err),
			Details: map[string]any{"path": p},
		}
	}
	return &t, nil
}

// Write serializes task into <taskDir>/.task.json, creating the file
// with 0o644. Caller owns the parent dir's existence.
func Write(taskDir string, t *Task) error {
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return &errs.TaskError{
			Code:    errs.Internal,
			Message: fmt.Sprintf("marshalling task: %v", err),
		}
	}
	p := filepath.Join(taskDir, MetadataFile)
	if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
		return &errs.TaskError{
			Code:    errs.Internal,
			Message: fmt.Sprintf("writing %s: %v", p, err),
			Details: map[string]any{"path": p},
		}
	}
	return nil
}
