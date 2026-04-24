package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jordangarrison/do-stuff/internal/config"
	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/task"
	"github.com/jordangarrison/do-stuff/internal/tmux"
)

// errPickCancelled is returned by a selectorFunc when the user aborts the picker.
var errPickCancelled = errors.New("pick: selection cancelled")

// selectorFunc accepts a slice of candidate slugs and returns the chosen one.
// Implementations may render UI; the default shells out to fzf.
type selectorFunc func(slugs []string) (string, error)

// lookupFunc shadows exec.LookPath for test injection.
type lookupFunc func(bin string) (string, error)

// NewPickCmd builds `ds pick` including the hidden --preview helper.
func NewPickCmd(flags *GlobalFlags) *cobra.Command {
	var previewSlug string
	cmd := &cobra.Command{
		Use:   "pick",
		Short: "fuzzy-select a task and attach to it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := DetectMode(DetectOpts{
				IsTerminal: IsStdoutTerminal(),
				JSON:       flags.JSON,
				Human:      flags.Human,
			})
			code := runPick(pickOpts{
				PreviewSlug: previewSlug,
				ConfigPath:  config.DefaultPath(),
				Mode:        mode,
				Stdout:      cmd.OutOrStdout(),
				Stderr:      cmd.ErrOrStderr(),
				LookupFn:    exec.LookPath,
				SelectorFn:  defaultFzfSelector,
				ExecFn:      syscall.Exec,
			})
			if code != 0 {
				return &ExitError{code: code}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&previewSlug, "preview", "", "internal: render preview for the given slug")
	_ = cmd.Flags().MarkHidden("preview")
	return cmd
}

type pickOpts struct {
	PreviewSlug string
	ConfigPath  string
	Mode        Mode
	Stdout      io.Writer
	Stderr      io.Writer
	LookupFn    lookupFunc
	SelectorFn  selectorFunc
	ExecFn      execFunc
}

// PickData is the success payload emitted when ds pick runs piped.
type PickData struct {
	Slug          string   `json:"slug"`
	Branch        string   `json:"branch"`
	Ticket        string   `json:"ticket,omitempty"`
	Repos         []string `json:"repos"`
	Session       string   `json:"session,omitempty"`
	SessionState  string   `json:"session_state"`
	AttachCommand string   `json:"attach_command,omitempty"`
}

func runPick(o pickOpts) int {
	cfg, err := config.Load(o.ConfigPath)
	if err != nil {
		return Render(RenderOpts{Command: "ds.pick", Err: err, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}
	if o.PreviewSlug != "" {
		return runPickPreview(o, cfg.TasksDir)
	}
	return runPickPrimary(o, cfg.TasksDir)
}

func runPickPreview(o pickOpts, tasksDir string) int {
	t, err := task.Load(filepath.Join(tasksDir, o.PreviewSlug))
	if err != nil {
		// Preview is invoked per-highlight by fzf; a hard failure would
		// kill the picker. Emit a note to stderr and exit 0.
		_, _ = fmt.Fprintf(o.Stderr, "preview: %s: %v\n", o.PreviewSlug, err)
		return 0
	}
	names := make([]string, 0, len(t.Repos))
	for _, r := range t.Repos {
		names = append(names, r.Name)
	}
	_, _ = fmt.Fprintf(o.Stdout, "slug:    %s\n", t.Slug)
	_, _ = fmt.Fprintf(o.Stdout, "type:    %s\n", t.Type)
	if t.Ticket != "" {
		_, _ = fmt.Fprintf(o.Stdout, "ticket:  %s\n", t.Ticket)
	}
	_, _ = fmt.Fprintf(o.Stdout, "branch:  %s\n", t.Branch)
	_, _ = fmt.Fprintf(o.Stdout, "base:    %s\n", t.Base)
	_, _ = fmt.Fprintf(o.Stdout, "repos:   %s\n", strings.Join(names, ", "))
	if t.TmuxSession != "" {
		state := probeSessionState(t.TmuxSession)
		_, _ = fmt.Fprintf(o.Stdout, "session: %s (%s)\n", t.TmuxSession, state)
	}
	return 0
}

func probeSessionState(session string) string {
	if err := tmux.Available(); err != nil {
		return "absent"
	}
	has, err := tmux.HasSession(session)
	if err != nil || !has {
		return "absent"
	}
	attached, err := tmux.IsSessionAttached(session)
	switch {
	case err != nil:
		return "detached"
	case attached:
		return "attached"
	default:
		return "detached"
	}
}

func runPickPrimary(o pickOpts, tasksDir string) int {
	var warn func(taskPath string, err error)
	if o.Mode == ModeJSON {
		warn = func(taskPath string, err error) {
			_, _ = fmt.Fprintf(o.Stderr, "warn: %s: %v\n", taskPath, err)
		}
	}
	tasks, err := loadAllTasks(tasksDir, warn)
	if err != nil {
		return Render(RenderOpts{Command: "ds.pick", Err: err, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}
	if len(tasks) == 0 {
		return Render(RenderOpts{
			Command: "ds.pick",
			Err: &errs.TaskError{
				Code:    errs.TaskNotFound,
				Message: fmt.Sprintf("no tasks found under %s", tasksDir),
				Details: map[string]any{"tasks_dir": tasksDir},
			},
			Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode,
		})
	}

	// fzf availability is checked even when a test injects SelectorFn, so
	// the pick_unavailable envelope stays reachable. Tests that don't want
	// to trip it stub LookupFn to return a non-empty path.
	lookup := o.LookupFn
	if lookup == nil {
		lookup = exec.LookPath
	}
	if _, err := lookup("fzf"); err != nil {
		return Render(RenderOpts{
			Command: "ds.pick",
			Err: &errs.TaskError{
				Code:    errs.PickUnavailable,
				Message: "fzf not found on PATH",
				Details: map[string]any{"binary": "fzf"},
			},
			Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode,
		})
	}

	slugs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		slugs = append(slugs, t.Slug)
	}
	selector := o.SelectorFn
	if selector == nil {
		selector = defaultFzfSelector
	}
	selected, err := selector(slugs)
	if err != nil {
		if errors.Is(err, errPickCancelled) {
			if o.Mode == ModeJSON {
				return Render(RenderOpts{
					Command: "ds.pick",
					Err: &errs.TaskError{
						Code:    errs.InvalidArgs,
						Message: "selection cancelled",
					},
					Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode,
				})
			}
			_, _ = fmt.Fprintln(o.Stderr, "pick: selection cancelled")
			return 1
		}
		return Render(RenderOpts{
			Command: "ds.pick",
			Err: &errs.TaskError{
				Code:    errs.Internal,
				Message: fmt.Sprintf("selector failed: %v", err),
			},
			Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode,
		})
	}

	picked, err := task.Load(filepath.Join(tasksDir, selected))
	if err != nil {
		return Render(RenderOpts{Command: "ds.pick", Err: err, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}
	data := buildPickData(picked)

	if o.Mode == ModeJSON {
		return Render(RenderOpts{Command: "ds.pick", Data: data, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}

	// TTY path: delegate to `ds attach <slug>` via exec. Preserves
	// session-recreate behavior without duplicating it here.
	// os.Executable resolves PATH/symlinks so syscall.Exec gets an
	// absolute binary path (execve does not search PATH).
	argv0, err := os.Executable()
	if err != nil {
		return Render(RenderOpts{
			Command: "ds.pick",
			Err: &errs.TaskError{
				Code:    errs.Internal,
				Message: fmt.Sprintf("resolving current executable: %v", err),
			},
			Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode,
		})
	}
	argv := []string{argv0, "attach", selected}
	if err := o.ExecFn(argv0, argv, os.Environ()); err != nil {
		return Render(RenderOpts{
			Command: "ds.pick",
			Err: &errs.TaskError{
				Code:    errs.Internal,
				Message: fmt.Sprintf("exec ds attach: %v", err),
			},
			Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode,
		})
	}
	return 0
}

func loadAllTasks(tasksDir string, warn func(taskPath string, err error)) ([]*task.Task, error) {
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
			// TTY picker stays silent so fzf preview pane is not
			// scrambled. Piped mode wires a warn closure so integrity
			// issues reach stderr the same way `ds list` surfaces them.
			if warn != nil {
				warn(taskPath, err)
			}
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func buildPickData(t *task.Task) PickData {
	names := make([]string, 0, len(t.Repos))
	for _, r := range t.Repos {
		names = append(names, r.Name)
	}
	d := PickData{
		Slug:         t.Slug,
		Branch:       t.Branch,
		Ticket:       t.Ticket,
		Repos:        names,
		SessionState: "absent",
	}
	if t.TmuxSession != "" {
		d.Session = t.TmuxSession
		d.AttachCommand = "tmux attach -t " + t.TmuxSession
		d.SessionState = probeSessionState(t.TmuxSession)
	}
	return d
}

func defaultFzfSelector(slugs []string) (string, error) {
	argv0, err := os.Executable()
	if err != nil {
		argv0 = os.Args[0]
	}
	preview := fmt.Sprintf(`%s pick --preview {}`, argv0)
	cmd := exec.Command("fzf",
		"--height=40%",
		"--reverse",
		"--no-sort",
		"--prompt=task> ",
		"--preview", preview,
		"--preview-window=right:50%",
	)
	cmd.Stdin = strings.NewReader(strings.Join(slugs, "\n") + "\n")
	cmd.Stderr = os.Stderr // UI to tty
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// fzf exit 130 = SIGINT (Ctrl-C/Esc); 1 = no match.
			_ = exitErr
			return "", errPickCancelled
		}
		return "", err
	}
	slug := strings.TrimSpace(string(out))
	if slug == "" {
		return "", errPickCancelled
	}
	return slug, nil
}
