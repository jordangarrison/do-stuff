package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jordangarrison/do-stuff/internal/config"
	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/task"
)

// NewAttachCmd builds `ds attach`. Positional arg is the slug.
func NewAttachCmd(flags *GlobalFlags) *cobra.Command {
	var startTmux bool
	cmd := &cobra.Command{
		Use:   "attach <slug>",
		Short: "attach to the task's tmux session, recreating it if the session has died",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := DetectMode(DetectOpts{
				IsTerminal: IsStdoutTerminal(),
				JSON:       flags.JSON,
				Human:      flags.Human,
			})
			code := runAttach(attachOpts{
				Slug:       args[0],
				StartTmux:  startTmux,
				ConfigPath: config.DefaultPath(),
				Mode:       mode,
				Stdout:     cmd.OutOrStdout(),
				Stderr:     cmd.ErrOrStderr(),
				AttachFn:   task.Attach,
				ExecFn:     syscall.Exec,
			})
			if code != 0 {
				return &ExitError{code: code}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&startTmux, "start-tmux", false, "create a tmux session for this task when metadata records none")
	return cmd
}

// execFunc matches the signature of syscall.Exec and is injectable for tests.
type execFunc func(argv0 string, argv []string, env []string) error

type attachOpts struct {
	Slug       string
	StartTmux  bool
	ConfigPath string
	Mode       Mode
	Stdout     io.Writer
	Stderr     io.Writer
	AttachFn   func(task.AttachParams) (*task.AttachResult, error)
	ExecFn     execFunc
}

// AttachData is the success payload for ds.attach.
type AttachData struct {
	Slug          string `json:"slug"`
	Session       string `json:"session"`
	WasRecreated  bool   `json:"was_recreated"`
	AttachCommand string `json:"attach_command"`
}

var attachSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func runAttach(o attachOpts) int {
	if !attachSlugRe.MatchString(o.Slug) {
		return Render(RenderOpts{
			Command: "ds.attach",
			Err: &errs.TaskError{
				Code:    errs.InvalidArgs,
				Message: "slug must match ^[a-z0-9][a-z0-9._-]*$",
				Details: map[string]any{"slug": o.Slug},
			},
			Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode,
		})
	}

	cfg, err := config.Load(o.ConfigPath)
	if err != nil {
		return Render(RenderOpts{Command: "ds.attach", Err: err, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}

	res, err := o.AttachFn(task.AttachParams{
		Slug:       o.Slug,
		TasksDir:   cfg.TasksDir,
		TmuxPrefix: cfg.TmuxPrefix,
		StartTmux:  o.StartTmux,
	})
	if err != nil {
		return Render(RenderOpts{Command: "ds.attach", Err: err, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}

	data := AttachData{
		Slug:          res.Task.Slug,
		Session:       res.SessionName,
		WasRecreated:  res.WasRecreated,
		AttachCommand: "tmux attach -t " + res.SessionName,
	}

	// Piped consumers want the envelope and no process replacement.
	if o.Mode == ModeJSON {
		return Render(RenderOpts{Command: "ds.attach", Data: data, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}

	// TTY path: replace this process with `tmux attach -t <session>`.
	// Fall through to rendering an error envelope if exec fails.
	tmuxBin, lerr := exec.LookPath("tmux")
	if lerr != nil {
		return Render(RenderOpts{
			Command: "ds.attach",
			Err: &errs.TaskError{
				Code:    errs.TmuxUnavailable,
				Message: "tmux not found on PATH at exec time",
			},
			Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode,
		})
	}
	argv := []string{"tmux", "attach", "-t", res.SessionName}
	if err := o.ExecFn(tmuxBin, argv, os.Environ()); err != nil {
		return Render(RenderOpts{
			Command: "ds.attach",
			Err: &errs.TaskError{
				Code:    errs.Internal,
				Message: fmt.Sprintf("exec tmux: %v", err),
			},
			Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode,
		})
	}
	return 0
}
