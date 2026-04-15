package main

import (
	"flag"
	"fmt"
)

var abcCommands commander

func init() {
	usage := `'src abc' is a tool that manages agentic batch changes on a Sourcegraph instance.

Usage:

	src abc command [command options]

The commands are:

	variables	manage workflow instance variables

Use "src abc [command] -h" for more information about a command.
`

	flagSet := flag.NewFlagSet("abc", flag.ExitOnError)
	handler := func(args []string) error {
		abcCommands.run(flagSet, "src abc", usage, args)
		return nil
	}

	commands = append(commands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}
