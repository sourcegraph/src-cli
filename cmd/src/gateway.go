package main

import (
	"flag"
	"fmt"
)

var gatewayCommands commander

func init() {
	usage := `'src gateway' interacts with Cody Gateway (directly or through a Sourcegraph instance).

Usage:

	src gateway command [command options]

The commands are:

	benchmark           runs benchmarks against Cody Gateway
	benchmark-stream    runs benchmarks against Cody Gateway code completion streaming endpoints

Use "src gateway [command] -h" for more information about a command.

`

	flagSet := flag.NewFlagSet("gateway", flag.ExitOnError)
	handler := func(args []string) error {
		gatewayCommands.run(flagSet, "src gateway", usage, args)
		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet:   flagSet,
		aliases:   []string{}, // No aliases for gateway command
		handler:   handler,
		usageFunc: func() { fmt.Println(usage) },
	})
}
