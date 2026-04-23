package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jordangarrison/do-stuff/internal/errs"
)

func TestRender_successJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	data := map[string]string{"slug": "foo"}

	code := Render(RenderOpts{
		Command: "ds.repos",
		Data:    data,
		Err:     nil,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Mode:    ModeJSON,
	})

	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if !env.OK || env.Command != "ds.repos" {
		t.Fatalf("bad envelope: %+v", env)
	}
}

func TestRender_taskErrorMapsToExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	te := &errs.TaskError{Code: errs.ConfigError, Message: "no config"}

	code := Render(RenderOpts{
		Command: "ds.repos",
		Err:     te,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Mode:    ModeJSON,
	})

	if code != 8 {
		t.Fatalf("want exit 8, got %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if env.OK || env.Error == nil || env.Error.Code != errs.ConfigError {
		t.Fatalf("bad envelope: %+v", env)
	}
}

func TestRender_plainErrorBecomesInternal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Render(RenderOpts{
		Command: "ds.repos",
		Err:     errors.New("oops"),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Mode:    ModeJSON,
	})

	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v", err)
	}
	if env.Error.Code != errs.Internal {
		t.Fatalf("want internal_error, got %s", env.Error.Code)
	}
}

func TestRender_humanModeSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Render(RenderOpts{
		Command: "ds.repos",
		Data:    map[string]string{"slug": "foo"},
		Stdout:  &stdout,
		Stderr:  &stderr,
		Mode:    ModeHuman,
	})

	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}
	if len(stdout.Bytes()) == 0 {
		t.Fatalf("human mode wrote nothing to stdout")
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err == nil && env.OK {
		t.Fatalf("human stdout unexpectedly parsed as success envelope")
	}
}

func TestRender_humanModeError_writesJSONToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	te := &errs.TaskError{Code: errs.RepoNotFound, Message: "missing"}

	code := Render(RenderOpts{
		Command: "ds.repos",
		Err:     te,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Mode:    ModeHuman,
	})

	if code != 3 {
		t.Fatalf("want exit 3, got %d", code)
	}
	if stderr.Len() == 0 {
		t.Fatalf("stderr empty on error")
	}
}

func TestDetectMode_jsonFlagWins(t *testing.T) {
	m := DetectMode(DetectOpts{IsTerminal: true, JSON: true, Human: false})
	if m != ModeJSON {
		t.Fatalf("want ModeJSON, got %s", m)
	}
}

func TestDetectMode_humanFlagWins(t *testing.T) {
	m := DetectMode(DetectOpts{IsTerminal: false, JSON: false, Human: true})
	if m != ModeHuman {
		t.Fatalf("want ModeHuman, got %s", m)
	}
}

func TestDetectMode_ttyIsHuman(t *testing.T) {
	m := DetectMode(DetectOpts{IsTerminal: true})
	if m != ModeHuman {
		t.Fatalf("want ModeHuman, got %s", m)
	}
}

func TestDetectMode_pipeIsJSON(t *testing.T) {
	m := DetectMode(DetectOpts{IsTerminal: false})
	if m != ModeJSON {
		t.Fatalf("want ModeJSON, got %s", m)
	}
}
