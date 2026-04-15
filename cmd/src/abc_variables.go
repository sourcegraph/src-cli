package main

import (
	"flag"
	"fmt"
)

var abcVariablesCommands commander

func init() {
	usage := `'src abc variables' is a tool that manages workflow instance variables on a Sourcegraph instance.

Usage:

	src abc variables command [command options]

The commands are:

	set	set or remove a workflow instance variable

Use "src abc variables [command] -h" for more information about a command.
`

	flagSet := flag.NewFlagSet("variables", flag.ExitOnError)
	handler := func(args []string) error {
		abcVariablesCommands.run(flagSet, "src abc variables", usage, args)
		return nil
	}

	abcCommands = append(abcCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}
