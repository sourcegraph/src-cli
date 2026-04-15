package main

import (
	"flag"
	"fmt"
)

var abcVariablesCommands commander

func init() {
	usage := `'src abc variables' is a tool that manages workflow instance variables on agentic batch changes.

Usage:

	src abc variables command [command options]

The commands are:

	set	    set workflow instance variables
	delete	delete workflow instance variables

Use "src abc variables [command] -h" for more information about a command.
`

	flagSet := flag.NewFlagSet("variables", flag.ExitOnError)
	usageFunc := func() {
		fmt.Println(usage)
	}
	handler := func(args []string) error {
		abcVariablesCommands.run(flagSet, "src abc variables", usage, args)
		return nil
	}

	abcCommands = append(abcCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
