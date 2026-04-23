package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestHandleExecuteError_nilReturnsZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := HandleExecuteError(nil, &stdout, &stderr, ModeJSON)
	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("nil err should write nothing; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestHandleExecuteError_exitErrorPassesThrough(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := HandleExecuteError(&ExitError{code: 8}, &stdout, &stderr, ModeJSON)
	if code != 8 {
		t.Fatalf("want exit 8, got %d", code)
	}
	// ExitError means the command already rendered its envelope; don't double-render.
	if stdout.Len() != 0 {
		t.Fatalf("should not re-render on ExitError; stdout=%q", stdout.String())
	}
}

func TestHandleExecuteError_frameworkErrorRendersInvalidArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := HandleExecuteError(errors.New(`unknown command "nope" for "ds"`), &stdout, &stderr, ModeJSON)
	if code != 2 {
		t.Fatalf("want exit 2 (invalid_args), got %d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.OK || env.Error == nil {
		t.Fatalf("bad envelope: %+v", env)
	}
	if env.Error.Code != "invalid_args" {
		t.Fatalf("want invalid_args, got %s", env.Error.Code)
	}
}
