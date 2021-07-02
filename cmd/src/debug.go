package main

import (
	"flag"
	"fmt"
)

func init() {
	flagSet := flag.NewFlagSet("debug", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), `'src debug' gathers and bundles debug data from a Sourcegraph deployment.

USAGE
  src [-v] debug
`)
	}

	handler := func(args []string) error {
		return flagSet.Parse(args)
	}
	// Register the command.
	commands = append(commands, &command{
		aliases:   []string{"debug-dump"},
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
