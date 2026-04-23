package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

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
			code := runRepos(reposOpts{
				ConfigPath: config.DefaultPath(),
				Mode:       mode,
				Stdout:     os.Stdout,
				Stderr:     os.Stderr,
			})
			if code != 0 {
				return &ExitError{code: code}
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

// runRepos is the core of ds repos. Tests call it directly to drive behavior
// without going through cobra Execute.
func runRepos(o reposOpts) int {
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
		hint := fmt.Sprintf(
			"mkdir -p %s && cat > %s <<'YAML'\nrepo_roots:\n  - ~/dev\nYAML",
			filepath.Dir(o.ConfigPath),
			o.ConfigPath,
		)
		return Render(RenderOpts{
			Command: "ds.repos",
			Err: &errs.TaskError{
				Code:    errs.ConfigError,
				Message: "repo_roots is empty. Seed " + o.ConfigPath + " with:\n\n" + hint,
				Details: map[string]any{
					"config_path": o.ConfigPath,
					"hint":        hint,
				},
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

// ExitError carries a non-zero exit code from a command's RunE back to
// main. Main checks for this type after cobra.Execute and calls os.Exit
// with the carried code. We can't use os.Exit inside RunE because that
// bypasses cobra's error handling and makes Execute-based tests unable
// to observe exit codes without terminating the process.
type ExitError struct {
	code int
}

func (e *ExitError) Error() string { return "" }

// Code returns the exit code carried by this error.
func (e *ExitError) Code() int { return e.code }

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
