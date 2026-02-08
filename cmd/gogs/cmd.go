package main

import (
	"strings"

	"github.com/urfave/cli/v3"
)

func stringFlag(name, value, usage string) *cli.StringFlag {
	parts := strings.SplitN(name, ", ", 2)
	f := &cli.StringFlag{
		Name:  parts[0],
		Value: value,
		Usage: usage,
	}
	if len(parts) > 1 {
		f.Aliases = []string{parts[1]}
	}
	return f
}

func boolFlag(name, usage string) *cli.BoolFlag {
	parts := strings.SplitN(name, ", ", 2)
	f := &cli.BoolFlag{
		Name:  parts[0],
		Usage: usage,
	}
	if len(parts) > 1 {
		f.Aliases = []string{parts[1]}
	}
	return f
}
