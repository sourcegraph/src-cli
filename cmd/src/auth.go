package main

import (
	"flag"
	"fmt"
)

var authCommands commander

func init() {
	usage := `'src auth' provides authentication-related helper commands.

Usage:

	src auth command [command options]

The commands are:

	token   prints the current authentication token or Authorization header

Use "src auth [command] -h" for more information about a command.
`

	flagSet := flag.NewFlagSet("auth", flag.ExitOnError)
	handler := func(args []string) error {
		authCommands.run(flagSet, "src auth", usage, args)
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
