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
	compose              generates docker outputs
	server               generates docker outputs
	

Use "src debug [command] -h" for more information about a subcommands.
src debug has access to flags on src -- Ex: src -v kube -o foo.zip

`

	flagSet := flag.NewFlagSet("debug", flag.ExitOnError)
	handler := func(args []string) error {
		debugCommands.run(flagSet, "src debug", usage, args)
		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet:   flagSet,
		aliases:   []string{},
		handler:   handler,
		usageFunc: func() { fmt.Println(usage) },
	})
}
