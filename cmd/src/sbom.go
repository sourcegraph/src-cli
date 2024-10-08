package main

import (
	"flag"
	"fmt"
)

var sbomCommands commander

func init() {
	usage := `'src sbom' fetches and verified SBOM (Software Bill of Materials) data for Sourcegraph containers.

Usage:

	src sbom command [command options]

The commands are:

	fetch                 fetch SBOMs for a released version of Sourcegraph
`
	flagSet := flag.NewFlagSet("sbom", flag.ExitOnError)
	handler := func(args []string) error {
		sbomCommands.run(flagSet, "src sbom", usage, args)
		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet: flagSet,
		aliases: []string{"sbom"},
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}
