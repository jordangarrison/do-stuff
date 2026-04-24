package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/task"
)

// scanTasks walks tasksDir, returning one *task.Task per subdirectory that
// contains a readable .task.json. warn (nullable) is called for entries
// that fail to load; the scan continues past them.
//
// A missing tasksDir returns (nil, nil). Other readdir failures return a
// ConfigError.
func scanTasks(tasksDir string, warn func(taskPath string, err error)) ([]*task.Task, error) {
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, &errs.TaskError{
			Code:    errs.ConfigError,
			Message: fmt.Sprintf("reading tasks_dir %s: %v", tasksDir, err),
			Details: map[string]any{"path": tasksDir},
		}
	}
	out := make([]*task.Task, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		taskPath := filepath.Join(tasksDir, e.Name())
		if _, err := os.Stat(filepath.Join(taskPath, task.MetadataFile)); err != nil {
			continue
		}
		t, err := task.Load(taskPath)
		if err != nil {
			if warn != nil {
				warn(taskPath, err)
			}
			continue
		}
		out = append(out, t)
	}
	return out, nil
}
