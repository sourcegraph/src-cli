package main

import (
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

var deepsearchCommand = clicompat.Wrap(&cli.Command{
	Name:        "deepsearch",
	Aliases:     []string{"ds"},
	Usage:       "interacts with Sourcegraph Deep Search",
	UsageText:   "src deepsearch [command options]",
	Description: deepsearchExamples,
	HideVersion: true,
	Commands: []*cli.Command{
		deepsearchAskCommand,
		deepsearchAddQuestionCommand,
		deepsearchGetCommand,
		deepsearchListCommand,
		deepsearchCancelCommand,
		deepsearchDeleteCommand,
	},
})

const deepsearchExamples = `'src deepsearch' interacts with the Sourcegraph Deep Search API.

Usage:

	src deepsearch command [command options]

The commands are:

	ask          starts a Deep Search conversation and waits for the answer
	add-question adds a follow-up question to a conversation
	get          gets a conversation
	list         lists conversation summaries
	cancel       cancels an in-progress conversation
	delete       permanently deletes a conversation

Use "src deepsearch [command] -h" for more information about a command.
`
