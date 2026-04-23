package main

import (
	"os"

	"github.com/jordangarrison/do-stuff/internal/cli"
)

func main() {
	root := cli.NewRootCmd(resolveVersion())
	err := root.Execute()
	mode := cli.DetectMode(cli.DetectOpts{IsTerminal: cli.IsStdoutTerminal()})
	code := cli.HandleExecuteError(err, os.Stdout, os.Stderr, mode)
	os.Exit(code)
}
