package main

import (
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

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
