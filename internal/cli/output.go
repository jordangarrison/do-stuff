package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

// Envelope is the structured response emitted by every ds command.
type Envelope struct {
	OK      bool            `json:"ok"`
	Command string          `json:"command"`
	Data    any             `json:"data,omitempty"`
	Error   *errs.TaskError `json:"error,omitempty"`
}

// Mode controls whether Render emits pretty text or the JSON envelope.
type Mode string

const (
	ModeHuman Mode = "human"
	ModeJSON  Mode = "json"
)

// DetectOpts feeds DetectMode. Tests pass IsTerminal directly; production
// callers derive it from term.IsTerminal(int(os.Stdout.Fd())).
type DetectOpts struct {
	IsTerminal bool
	JSON       bool
	Human      bool
}

// DetectMode resolves the effective output mode: explicit flags win, otherwise
// a TTY on stdout means human and a pipe/redirect means JSON.
func DetectMode(o DetectOpts) Mode {
	switch {
	case o.JSON:
		return ModeJSON
	case o.Human:
		return ModeHuman
	case o.IsTerminal:
		return ModeHuman
	default:
		return ModeJSON
	}
}

// IsStdoutTerminal reports whether the current process's stdout is a TTY.
// Production entry point for DetectOpts.IsTerminal.
func IsStdoutTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// RenderOpts bundles everything Render needs. Stdout/Stderr are parameterized
// so tests can assert on captured buffers without touching global state.
type RenderOpts struct {
	Command string
	Data    any
	Err     error
	Stdout  io.Writer
	Stderr  io.Writer
	Mode    Mode
}

// Render writes the envelope (or human rendering) and returns the exit code.
// Callers are expected to `os.Exit(code)` on the return value.
func Render(o RenderOpts) int {
	env := Envelope{Command: o.Command}
	exitCode := 0

	if o.Err != nil {
		var te *errs.TaskError
		if !errors.As(o.Err, &te) {
			te = &errs.TaskError{
				Code:    errs.Internal,
				Message: o.Err.Error(),
			}
		}
		env.OK = false
		env.Error = te
		exitCode = te.ExitCode()
	} else {
		env.OK = true
		env.Data = o.Data
	}

	switch o.Mode {
	case ModeJSON:
		writeJSON(o.Stdout, env)
		if env.Error != nil {
			_, _ = fmt.Fprintf(o.Stderr, "error: %s (%s)\n", env.Error.Message, env.Error.Code)
		}
	case ModeHuman:
		if env.Error != nil {
			_, _ = fmt.Fprintf(o.Stdout, "error: %s\n", env.Error.Message)
			writeJSON(o.Stderr, env)
		} else {
			writeHumanSuccess(o.Stdout, env)
		}
	}

	return exitCode
}

func writeJSON(w io.Writer, env Envelope) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(env)
}

// writeHumanSuccess emits a terse human-friendly summary. Commands with
// richer rendering can override by using ModeJSON and formatting themselves;
// for the v0.1 slice 1 default this pretty-prints the data block.
func writeHumanSuccess(w io.Writer, env Envelope) {
	_, _ = fmt.Fprintf(w, "ok: %s\n", env.Command)
	if env.Data != nil {
		b, err := json.MarshalIndent(env.Data, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintf(w, "  (data unmarshalable: %v)\n", err)
			return
		}
		_, _ = fmt.Fprintln(w, string(b))
	}
}
