package main

import (
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

var orgsMembersCommands commander

var orgsMembersCommand = clicompat.Wrap(&cli.Command{
	Name:        "members",
	Aliases:     []string{"member"},
	Usage:       "manages organization members",
	UsageText:   "src orgs members [command options]",
	Description: orgsMembersExamples,
	HideVersion: true,
	Commands: []*cli.Command{
		orgsMembersAddCommand,
		orgsMembersRemoveCommand,
	},
})

const orgsMembersExamples = `'src orgs members' is a tool that manages organization members on a Sourcegraph instance.

Usage:

	src orgs members command [command options]

The commands are:

	add        adds a user as a member to an organization
	remove     removes a user as a member from an organization

Use "src orgs members [command] -h" for more information about a command.
`

func init() {
	usage := orgsMembersExamples

	flagSet := flag.NewFlagSet("members", flag.ExitOnError)
	handler := func(args []string) error {
		orgsMembersCommands.run(flagSet, "src orgs members", usage, args)
		return nil
	}

	// Register the command.
	orgsCommands = append(orgsCommands, &command{
		flagSet: flagSet,
		aliases: []string{"member"},
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}
