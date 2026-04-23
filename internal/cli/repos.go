package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/jordangarrison/do-stuff/internal/config"
	"github.com/jordangarrison/do-stuff/internal/discover"
	"github.com/jordangarrison/do-stuff/internal/errs"
)

// NewReposCmd builds `ds repos`. Reads config, walks roots, emits envelope.
func NewReposCmd(flags *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "repos",
		Short: "list configured repo roots and discovered repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := DetectMode(DetectOpts{
				IsTerminal: IsStdoutTerminal(),
				JSON:       flags.JSON,
				Human:      flags.Human,
			})
			code := runReposForTest(reposOpts{
				ConfigPath: config.DefaultPath(),
				Mode:       mode,
				Stdout:     os.Stdout,
				Stderr:     os.Stderr,
			})
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
}

// reposOpts isolates the logic from global state so tests can drive it.
type reposOpts struct {
	ConfigPath string
	Mode       Mode
	Stdout     io.Writer
	Stderr     io.Writer
}

// runReposForTest is the testable core of `ds repos`. Returns the exit code
// (also written via Render). Kept exported-in-test via the same package.
func runReposForTest(o reposOpts) int {
	cfg, err := config.Load(o.ConfigPath)
	if err != nil {
		return Render(RenderOpts{
			Command: "ds.repos",
			Err:     err,
			Stdout:  o.Stdout,
			Stderr:  o.Stderr,
			Mode:    o.Mode,
		})
	}

	if len(cfg.RepoRoots) == 0 {
		return Render(RenderOpts{
			Command: "ds.repos",
			Err: &errs.TaskError{
				Code:    errs.ConfigError,
				Message: "repo_roots is empty; set it in " + o.ConfigPath,
				Details: map[string]any{"config_path": o.ConfigPath},
			},
			Stdout: o.Stdout,
			Stderr: o.Stderr,
			Mode:   o.Mode,
		})
	}

	repos, err := discover.Walk(cfg.RepoRoots)
	if err != nil {
		return Render(RenderOpts{
			Command: "ds.repos",
			Err:     err,
			Stdout:  o.Stdout,
			Stderr:  o.Stderr,
			Mode:    o.Mode,
		})
	}

	return Render(RenderOpts{
		Command: "ds.repos",
		Data:    marshalReposData(repos, cfg.RepoRoots),
		Stdout:  o.Stdout,
		Stderr:  o.Stderr,
		Mode:    o.Mode,
	})
}

func marshalReposData(repos []discover.Repo, roots []string) map[string]any {
	r := make([]map[string]any, 0, len(repos))
	for _, repo := range repos {
		r = append(r, map[string]any{
			"name": repo.Name,
			"path": repo.Path,
			"root": repo.Root,
		})
	}
	return map[string]any{
		"repos": r,
		"roots": roots,
	}
}
