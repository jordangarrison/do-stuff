package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jordangarrison/do-stuff/internal/config"
	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/task"
)

// NewListCmd builds `ds list`.
func NewListCmd(flags *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list tasks discovered in the configured tasks_dir",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := DetectMode(DetectOpts{
				IsTerminal: IsStdoutTerminal(),
				JSON:       flags.JSON,
				Human:      flags.Human,
			})
			code := runList(listOpts{
				ConfigPath: config.DefaultPath(),
				Mode:       mode,
				Stdout:     cmd.OutOrStdout(),
				Stderr:     cmd.ErrOrStderr(),
			})
			if code != 0 {
				return &ExitError{code: code}
			}
			return nil
		},
	}
}

type listOpts struct {
	ConfigPath string
	Mode       Mode
	Stdout     io.Writer
	Stderr     io.Writer
}

// ListData is the success payload for ds.list.
type ListData struct {
	Tasks []ListTask `json:"tasks"`
}

// ListTask is one entry. Repos is a flat []string of names (not the full
// RepoRef; see SPEC envelope shape).
type ListTask struct {
	Slug         string   `json:"slug"`
	Type         string   `json:"type"`
	Ticket       string   `json:"ticket,omitempty"`
	Branch       string   `json:"branch"`
	Repos        []string `json:"repos"`
	Session      string   `json:"session,omitempty"`
	SessionState string   `json:"session_state"`
	CreatedAt    string   `json:"created_at"`
}

func runList(o listOpts) int {
	cfg, err := config.Load(o.ConfigPath)
	if err != nil {
		return Render(RenderOpts{Command: "ds.list", Err: err, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}

	entries, err := os.ReadDir(cfg.TasksDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Render(RenderOpts{
			Command: "ds.list",
			Err: &errs.TaskError{
				Code:    errs.ConfigError,
				Message: fmt.Sprintf("reading tasks_dir %s: %v", cfg.TasksDir, err),
				Details: map[string]any{"path": cfg.TasksDir},
			},
			Stdout: o.Stdout,
			Stderr: o.Stderr,
			Mode:   o.Mode,
		})
	}
	tasks := make([]ListTask, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		taskPath := filepath.Join(cfg.TasksDir, e.Name())
		if _, err := os.Stat(filepath.Join(taskPath, task.MetadataFile)); err != nil {
			continue
		}
		t, err := task.Load(taskPath)
		if err != nil {
			_, _ = fmt.Fprintf(o.Stderr, "warn: %s: %v\n", taskPath, err)
			continue
		}
		tasks = append(tasks, buildListTask(t))
	}

	return Render(RenderOpts{
		Command: "ds.list",
		Data:    ListData{Tasks: tasks},
		Stdout:  o.Stdout,
		Stderr:  o.Stderr,
		Mode:    o.Mode,
	})
}

func buildListTask(t *task.Task) ListTask {
	names := make([]string, 0, len(t.Repos))
	for _, r := range t.Repos {
		names = append(names, r.Name)
	}
	state := "absent"
	if t.TmuxSession != "" {
		state = probeSessionState(t.TmuxSession)
	}
	return ListTask{
		Slug:         t.Slug,
		Type:         t.Type,
		Ticket:       t.Ticket,
		Branch:       t.Branch,
		Repos:        names,
		Session:      t.TmuxSession,
		SessionState: state,
		CreatedAt:    t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
