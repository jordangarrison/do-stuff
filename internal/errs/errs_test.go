package errs

import (
	"errors"
	"testing"
)

func TestExitCode_table(t *testing.T) {
	cases := []struct {
		code Code
		want int
	}{
		{InvalidArgs, 2},
		{RepoNotFound, 3},
		{TaskExists, 4},
		{BranchConflict, 5},
		{WorktreeExists, 5},
		{TmuxUnavailable, 6},
		{TmuxSessionExists, 6},
		{TmuxSessionMissing, 6},
		{GitError, 7},
		{WorktreeDirty, 7},
		{ConfigError, 8},
		{TaskNotFound, 9},
		{Internal, 1},
		{PickUnavailable, 2},
		{WorktreeMissing, 5},
	}
	for _, c := range cases {
		t.Run(string(c.code), func(t *testing.T) {
			got := (&TaskError{Code: c.code}).ExitCode()
			if got != c.want {
				t.Fatalf("code %s: want %d, got %d", c.code, c.want, got)
			}
		})
	}
}

func TestExitCode_unknownCodeIsOne(t *testing.T) {
	got := (&TaskError{Code: "bogus_not_a_real_code"}).ExitCode()
	if got != 1 {
		t.Fatalf("want 1, got %d", got)
	}
}

func TestError_implementsError(t *testing.T) {
	var err error = &TaskError{Code: InvalidArgs, Message: "bad flag"}
	if err.Error() != "bad flag" {
		t.Fatalf("want 'bad flag', got %q", err.Error())
	}
}

func TestError_errorsAsWorks(t *testing.T) {
	var err error = &TaskError{Code: ConfigError, Message: "no config"}
	var te *TaskError
	if !errors.As(err, &te) {
		t.Fatalf("errors.As failed to unwrap TaskError")
	}
	if te.Code != ConfigError {
		t.Fatalf("want ConfigError, got %s", te.Code)
	}
}
