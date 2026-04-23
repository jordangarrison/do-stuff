package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jordangarrison/do-stuff/internal/config"
	"github.com/jordangarrison/do-stuff/internal/discover"
	"github.com/jordangarrison/do-stuff/internal/errs"
)

// ReposData is the success payload for `ds repos`. Fields carry JSON tags
// matching the documented envelope.
type ReposData struct {
	Repos []RepoItem `json:"repos"`
	Roots []string   `json:"roots"`
}

// RepoItem is one discovered repo. Root is the configured root the repo was
// found under (useful for disambiguation when names collide across roots).
type RepoItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Root string `json:"root"`
}

// NewReposCmd builds `ds repos`. Reads config, walks roots, emits envelope.
func NewReposCmd(flags *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "repos",
		Short: "list configured repo roots and discovered repositories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := DetectMode(DetectOpts{
				IsTerminal: IsStdoutTerminal(),
				JSON:       flags.JSON,
				Human:      flags.Human,
			})
			code := runRepos(reposOpts{
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

// reposOpts isolates the logic from global state so tests can drive it.
type reposOpts struct {
	ConfigPath string
	Mode       Mode
	Stdout     io.Writer
	Stderr     io.Writer
}

// runRepos is the core of ds repos. Tests call it directly.
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
		Data:    buildReposData(repos, cfg.RepoRoots),
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

func buildReposData(repos []discover.Repo, roots []string) ReposData {
	items := make([]RepoItem, 0, len(repos))
	for _, repo := range repos {
		items = append(items, RepoItem{
			Name: repo.Name,
			Path: repo.Path,
			Root: repo.Root,
		})
	}
	return ReposData{Repos: items, Roots: roots}
}
