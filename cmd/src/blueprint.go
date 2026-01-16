package main

import (
	"flag"
	"fmt"
)

const defaultBlueprintRepo = "https://github.com/sourcegraph-community/blueprints"

var blueprintCommands commander

func init() {
	usage := `INTERNAL USE ONLY: 'src blueprint' manages blueprints on a Sourcegraph instance.

Usage:
	src blueprint command [command options]

The commands are:

	list                 lists blueprints from a remote repository or local path
	import               imports blueprints from a remote repository or local path

Use "src blueprint [command] -h" for more information about a command.

`

	flagset := flag.NewFlagSet("blueprint", flag.ExitOnError)
	handler := func(args []string) error {
		blueprintCommands.run(flagset, "src blueprint", usage, args)
		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet:   flagset,
		handler:   handler,
		usageFunc: func() { fmt.Println(usage) },
	})
}
