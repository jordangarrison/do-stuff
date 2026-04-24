package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/jordangarrison/do-stuff/internal/config"
	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/task"
)

// NewFinishCmd builds `ds finish`. Positional arg is the slug.
func NewFinishCmd(flags *GlobalFlags) *cobra.Command {
	var (
		force        bool
		keepBranches bool
	)
	cmd := &cobra.Command{
		Use:   "finish <slug>",
		Short: "tear down a task: remove worktrees, kill tmux session, optionally delete branches",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := DetectMode(DetectOpts{
				IsTerminal: IsStdoutTerminal(),
				JSON:       flags.JSON,
				Human:      flags.Human,
			})
			code := runFinish(finishOpts{
				Slug:         args[0],
				Force:        force,
				KeepBranches: keepBranches,
				ConfigPath:   config.DefaultPath(),
				Mode:         mode,
				Stdout:       cmd.OutOrStdout(),
				Stderr:       cmd.ErrOrStderr(),
				FinishFn:     task.Finish,
			})
			if code != 0 {
				return &ExitError{code: code}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "discard dirty worktrees and delete unmerged branches")
	cmd.Flags().BoolVar(&keepBranches, "keep-branches", false, "do not delete branches from the source repos")
	return cmd
}

type finishOpts struct {
	Slug         string
	Force        bool
	KeepBranches bool
	ConfigPath   string
	Mode         Mode
	Stdout       io.Writer
	Stderr       io.Writer
	FinishFn     func(task.FinishParams) (*task.FinishResult, error)
}

// FinishData is the success payload for ds.finish.
type FinishData struct {
	Slug             string   `json:"slug"`
	RemovedWorktrees []string `json:"removed_worktrees"`
	KilledSession    string   `json:"killed_session,omitempty"`
	BranchesKept     bool     `json:"branches_kept"`
}

func runFinish(o finishOpts) int {
	if !taskSlugRe.MatchString(o.Slug) {
		return Render(RenderOpts{
			Command: "ds.finish",
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
		return Render(RenderOpts{Command: "ds.finish", Err: err, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}

	res, err := o.FinishFn(task.FinishParams{
		Slug:         o.Slug,
		TasksDir:     cfg.TasksDir,
		Force:        o.Force,
		KeepBranches: o.KeepBranches,
	})
	if err != nil {
		return Render(RenderOpts{Command: "ds.finish", Err: err, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}

	return Render(RenderOpts{
		Command: "ds.finish",
		Data: FinishData{
			Slug:             res.Task.Slug,
			RemovedWorktrees: res.RemovedWorktrees,
			KilledSession:    res.KilledSession,
			BranchesKept:     res.BranchesKept,
		},
		Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode,
	})
}
