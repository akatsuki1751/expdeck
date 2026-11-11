package main

import (
	"os"

	"expdeck/internal/cli"
)

func main() {
	os.Exit(cli.Run(cli.Env{
		Dir:    ".",
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		IsTTY:  isTerminal(),
	}, os.Args[1:]))
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
