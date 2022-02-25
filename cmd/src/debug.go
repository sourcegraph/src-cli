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

/*
TODO:
	- This project needs some consideration around monitoring
		- You should be aware when a executed cmd has failed
		- You should be able to recieve an output that tells you what you've created in the zip file
		- an additional introspection command might be useful to look at whats in a zip file easily
*/
