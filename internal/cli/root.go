package cli

import (
	"github.com/spf13/cobra"
)

// GlobalFlags holds persistent flags threaded through every subcommand.
type GlobalFlags struct {
	JSON  bool
	Human bool
}

// NewRootCmd constructs the `ds` root command with all v0.1 subcommands
// registered. The version string is injected by main (post-resolveVersion).
func NewRootCmd(version string) *cobra.Command {
	flags := &GlobalFlags{}

	root := &cobra.Command{
		Use:           "ds",
		Short:         "do-stuff: task-based multi-repo worktree manager",
		Version:       version,
		SilenceUsage:  true, // envelope already reports errors
		SilenceErrors: true,
	}
	root.SetVersionTemplate("ds {{.Version}}\n")

	root.PersistentFlags().BoolVar(&flags.JSON, "json", false, "force JSON envelope output")
	root.PersistentFlags().BoolVar(&flags.Human, "human", false, "force human-readable output")

	root.AddCommand(NewReposCmd(flags))

	return root
}
