package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jordangarrison/do-stuff/internal/config"
	"github.com/jordangarrison/do-stuff/internal/discover"
	"github.com/jordangarrison/do-stuff/internal/errs"
	"github.com/jordangarrison/do-stuff/internal/task"
)

// NewNewCmd builds `ds new`. Positional arg is the slug.
func NewNewCmd(flags *GlobalFlags) *cobra.Command {
	var (
		reposCSV string
		typ      string
		ticket   string
		branch   string
		base     string
		noTmux   bool
		strict   bool
	)
	cmd := &cobra.Command{
		Use:   "new <slug>",
		Short: "create a new task with one worktree per repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := DetectMode(DetectOpts{
				IsTerminal: IsStdoutTerminal(),
				JSON:       flags.JSON,
				Human:      flags.Human,
			})
			code := runNew(newOpts{
				Slug:           args[0],
				Type:           typ,
				Ticket:         ticket,
				BranchOverride: branch,
				Base:           base,
				Repos:          splitCSV(reposCSV),
				NoTmux:         noTmux,
				Strict:         strict,
				ConfigPath:     config.DefaultPath(),
				Mode:           mode,
				Stdout:         cmd.OutOrStdout(),
				Stderr:         cmd.ErrOrStderr(),
			})
			if code != 0 {
				return &ExitError{code: code}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&reposCSV, "repos", "", "comma-separated repo names (required)")
	cmd.Flags().StringVar(&typ, "type", "", "conventional-commit type; defaults to config default_type")
	cmd.Flags().StringVar(&ticket, "ticket", "", "ticket id embedded in branch name and metadata")
	cmd.Flags().StringVar(&branch, "branch", "", "override the derived branch name entirely")
	cmd.Flags().StringVar(&base, "base", "", "base branch to start from; defaults to config default_base")
	cmd.Flags().BoolVar(&noTmux, "no-tmux", false, "skip tmux session creation")
	cmd.Flags().BoolVar(&strict, "strict", false, "fail if the target branch already exists locally or on origin")
	_ = cmd.MarkFlagRequired("repos")
	return cmd
}

type newOpts struct {
	Slug           string
	Type           string
	Ticket         string
	BranchOverride string
	Base           string
	Repos          []string
	NoTmux         bool
	Strict         bool
	ConfigPath     string
	Mode           Mode
	Stdout         io.Writer
	Stderr         io.Writer
}

var (
	slugRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	validTypes = map[string]struct{}{"feat": {}, "fix": {}, "chore": {}, "refactor": {}, "docs": {}, "test": {}, "perf": {}, "build": {}, "ci": {}}
)

func runNew(o newOpts) int {
	cfg, err := config.Load(o.ConfigPath)
	if err != nil {
		return Render(RenderOpts{Command: "ds.new", Err: err, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}

	// Apply config defaults.
	typ := o.Type
	if typ == "" {
		typ = cfg.DefaultType
	}
	base := o.Base
	if base == "" {
		base = cfg.DefaultBase
	}

	if !slugRe.MatchString(o.Slug) {
		return renderErr(o, errs.InvalidArgs, "slug must match ^[a-z0-9][a-z0-9._-]*$", map[string]any{"slug": o.Slug})
	}
	if _, ok := validTypes[typ]; !ok {
		return renderErr(o, errs.InvalidArgs, fmt.Sprintf("invalid type %q", typ), map[string]any{"type": typ})
	}
	if len(o.Repos) == 0 {
		return renderErr(o, errs.InvalidArgs, "--repos requires at least one repo", nil)
	}
	if o.BranchOverride != "" && strings.TrimSpace(o.BranchOverride) == "" {
		return renderErr(o, errs.InvalidArgs, "--branch cannot be blank", nil)
	}

	if len(cfg.RepoRoots) == 0 {
		return renderErr(o, errs.ConfigError, "repo_roots is empty; see `ds repos` for the config hint", map[string]any{"config_path": o.ConfigPath})
	}
	repos, err := discover.Walk(cfg.RepoRoots)
	if err != nil {
		return Render(RenderOpts{Command: "ds.new", Err: err, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}
	byName := make(map[string]discover.Repo, len(repos))
	names := make([]string, 0, len(repos))
	for _, r := range repos {
		byName[r.Name] = r
		names = append(names, r.Name)
	}
	resolved := make([]task.ResolvedRepo, 0, len(o.Repos))
	for _, name := range o.Repos {
		r, ok := byName[name]
		if !ok {
			return renderErr(o, errs.RepoNotFound,
				fmt.Sprintf("no repo named %q under configured roots", name),
				map[string]any{"requested": name, "available": names},
			)
		}
		resolved = append(resolved, task.ResolvedRepo{Name: r.Name, Path: r.Path})
	}

	res, err := task.Create(task.CreateParams{
		Slug:           o.Slug,
		Type:           typ,
		Ticket:         o.Ticket,
		BranchOverride: o.BranchOverride,
		Base:           base,
		TasksDir:       cfg.TasksDir,
		Repos:          resolved,
		NoTmux:         o.NoTmux,
		StartTmux:      cfg.StartTmux,
		Strict:         o.Strict,
		TmuxPrefix:     cfg.TmuxPrefix,
	})
	if err != nil {
		return Render(RenderOpts{Command: "ds.new", Err: err, Stdout: o.Stdout, Stderr: o.Stderr, Mode: o.Mode})
	}

	return Render(RenderOpts{
		Command: "ds.new",
		Data:    buildNewData(res),
		Stdout:  o.Stdout,
		Stderr:  o.Stderr,
		Mode:    o.Mode,
	})
}

// NewData is the success payload for ds.new.
type NewData struct {
	Slug          string        `json:"slug"`
	Path          string        `json:"path"`
	Branch        string        `json:"branch"`
	Base          string        `json:"base"`
	Ticket        string        `json:"ticket,omitempty"`
	Repos         []NewRepoData `json:"repos"`
	TmuxSession   string        `json:"tmux_session,omitempty"`
	AttachCommand string        `json:"attach_command,omitempty"`
}

// NewRepoData is one repo entry inside NewData.Repos.
type NewRepoData struct {
	Name         string `json:"name"`
	WorktreePath string `json:"worktree_path"`
	BranchState  string `json:"branch_state"`
}

func buildNewData(r *task.CreateResult) NewData {
	repos := make([]NewRepoData, 0, len(r.RepoStates))
	for _, s := range r.RepoStates {
		repos = append(repos, NewRepoData{Name: s.Name, WorktreePath: s.WorktreePath, BranchState: s.BranchState})
	}
	d := NewData{
		Slug:   r.Task.Slug,
		Path:   r.TaskDir,
		Branch: r.Task.Branch,
		Base:   r.Task.Base,
		Ticket: r.Task.Ticket,
		Repos:  repos,
	}
	if r.Task.TmuxSession != "" {
		d.TmuxSession = r.Task.TmuxSession
		d.AttachCommand = "tmux attach -t " + r.Task.TmuxSession
	}
	return d
}

func renderErr(o newOpts, code errs.Code, msg string, details map[string]any) int {
	return Render(RenderOpts{
		Command: "ds.new",
		Err:     &errs.TaskError{Code: code, Message: msg, Details: details},
		Stdout:  o.Stdout,
		Stderr:  o.Stderr,
		Mode:    o.Mode,
	})
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
