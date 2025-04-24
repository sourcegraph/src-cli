package main

import (
	"flag"
	"fmt"
)

var promptsCommands commander

func init() {
	usage := `'src prompts' is a tool that manages prompt library prompts and tags in a Sourcegraph instance.

Usage:

	src prompts command [command options]

The commands are:

	list		lists prompts
	get		    get a prompt by ID
	create		create a prompt
	update		update a prompt
	delete		delete a prompt
	export		export prompts to a JSON file
	import		import prompts from a JSON file
	tags		manage prompt tags (use "src prompts tags [command] -h" for more info)

Use "src prompts [command] -h" for more information about a command.
`

	flagSet := flag.NewFlagSet("prompts", flag.ExitOnError)
	handler := func(args []string) error {
		promptsCommands.run(flagSet, "src prompts", usage, args)
		return nil
	}

	// Register the command.
	commands = append(commands, &command{
		flagSet: flagSet,
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}

const promptFragment = `
fragment PromptFields on Prompt {
	id
	name
	description
	definition {
		text
	}
	draft
	visibility
	autoSubmit
	mode
	recommended
	tags(first: 100) {
		nodes {
			id
			name
		}
	}
}
`

const promptTagFragment = `
fragment PromptTagFields on PromptTag {
	id
	name
}
`

type Prompt struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Definition  Definition `json:"definition"`
	Draft       bool       `json:"draft"`
	Visibility  string     `json:"visibility"`
	AutoSubmit  bool       `json:"autoSubmit"`
	Mode        string     `json:"mode"`
	Recommended bool       `json:"recommended"`
	Tags        PromptTags `json:"tags"`
}

type Definition struct {
	Text string `json:"text"`
}

type PromptTags struct {
	Nodes []PromptTag `json:"nodes"`
}

type PromptTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
