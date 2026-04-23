package main

import (
	"os"

	"github.com/jordangarrison/do-stuff/internal/cli"
)

func main() {
	root := cli.NewRootCmd(resolveVersion())
	if err := root.Execute(); err != nil {
		// Cobra returns errors bare. Actual envelope rendering happens
		// inside each RunE via cli.Render, so we only reach here on
		// unrecoverable framework errors. Exit non-zero.
		os.Exit(1)
	}
}
