package main

import (
	"flag"
	"fmt"
)

var reposTagsCommands commander

func init() {
	usage := `'src repos tags' allows management of repository tags.

Usage:

	src repos tags command [command options]

The commands are:

	add        adds a tag to a repository
	delete     deletes a tag from a repository
	list       lists tags on a repository

Use "src repos tags [command] -h" for more information about a command.
`

	flagSet := flag.NewFlagSet("tags", flag.ExitOnError)

	handler := func(args []string) error {
		reposTagsCommands.run(flagSet, "src repos tags", usage, args)
		return nil
	}

	// Register the command.
	reposCommands = append(reposCommands, &command{
		flagSet: flagSet,
		aliases: []string{"tag"},
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}
