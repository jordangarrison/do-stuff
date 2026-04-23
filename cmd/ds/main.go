package main

import (
	"errors"
	"os"

	"github.com/jordangarrison/do-stuff/internal/cli"
)

func main() {
	root := cli.NewRootCmd(resolveVersion())
	err := root.Execute()
	if err == nil {
		return
	}
	var exitErr *cli.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.Code())
	}
	// Cobra returns errors bare for framework-level failures. Actual
	// envelope rendering happens inside each RunE via cli.Render, so
	// we only reach here on unrecoverable framework errors.
	os.Exit(1)
}
