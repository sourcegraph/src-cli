package main

import (
	"flag"
	"fmt"
)

var promptsTagsCommands commander

func init() {
	usage := `'src prompts tags' is a tool that manages prompt tags in a Sourcegraph instance.

Usage:

	src prompts tags command [command options]

The commands are:

	list        lists prompt tags
	get         get a prompt tag by name
	create      create a prompt tag
	update      update a prompt tag
	delete      delete a prompt tag

Use "src prompts tags [command] -h" for more information about a command.
`

	flagSet := flag.NewFlagSet("tags", flag.ExitOnError)
	handler := func(args []string) error {
		promptsTagsCommands.run(flagSet, "src prompts tags", usage, args)
		return nil
	}

	// Register the command.
	promptsCommands = append(promptsCommands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}
