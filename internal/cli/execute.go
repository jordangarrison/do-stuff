package cli

import (
	"errors"
	"io"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

// HandleExecuteError translates the error returned by cobra.Command.Execute
// into a process exit code, honoring the envelope contract. Commands signal
// structured failures with *ExitError (already rendered); framework failures
// (unknown subcommand, bad flag, Args validator rejection) arrive as bare
// errors and get rendered here as invalid_args so piped consumers never see
// an empty envelope.
func HandleExecuteError(err error, stdout, stderr io.Writer, mode Mode) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code()
	}
	return Render(RenderOpts{
		Command: "ds",
		Err: &errs.TaskError{
			Code:    errs.InvalidArgs,
			Message: err.Error(),
		},
		Stdout: stdout,
		Stderr: stderr,
		Mode:   mode,
	})
}
