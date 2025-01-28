package main

import (
	"flag"
	"fmt"
)

var signatureCommands commander

func init() {
	usage := `'src signature' verifies published signatures for Sourcegraph containers.

Usage:

	src signature command [command options]

The commands are:

	verify                 verify signatures for a released version of Sourcegraph
`
	flagSet := flag.NewFlagSet("signature", flag.ExitOnError)
	handler := func(args []string) error {
		signatureCommands.run(flagSet, "src signature", usage, args)
		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet: flagSet,
		aliases: []string{"signature", "sig"},
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}
