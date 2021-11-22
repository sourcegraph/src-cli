package main

import (
	"flag"
	"fmt"
)

var debugCommands commander

func init() {
	usage := `'src debug' gathers and bundles debug data from a Sourcegraph deployment for troubleshooting.

Usage:

	src debug command [command options]

The commands are:

	kube                 generates kubectl outputs
	comp                 generates docker outputs
	serv                 generates docker outputs
	

Use "src debug [command] -h" for more information about a command.

`

	flagSet := flag.NewFlagSet("debug", flag.ExitOnError)
	handler := func(args []string) error {
		debugCommands.run(flagSet, "src debug", usage, args)
		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet: flagSet,
		aliases: []string{
			"debug-dump",
			"debugger",
		},
		handler:   handler,
		usageFunc: func() { fmt.Println(usage) },
	})
}
