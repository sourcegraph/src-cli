package main

import (
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

var orgsCommand = clicompat.Wrap(&cli.Command{
	Name:        "orgs",
	Aliases:     []string{"org"},
	Usage:       "manages organizations",
	UsageText:   "src orgs [command options]",
	Description: orgsExamples,
	HideVersion: true,
	Commands: []*cli.Command{
		orgsListCommand,
		orgsGetCommand,
		orgsCreateCommand,
		orgsDeleteCommand,
		orgsMembersCommand,
	},
})

const orgsExamples = `'src orgs' is a tool that manages organizations on a Sourcegraph instance.

Usage:

	src orgs command [command options]

The commands are:

	list       lists organizations
	get        gets an organization
	create     creates an organization
	delete     deletes an organization
	members    manages organization members

Use "src orgs [command] -h" for more information about a command.
`

const orgFragment = `
fragment OrgFields on Org {
    id
    name
    displayName
    members {
        nodes {
			id
			username
		}
    }
}
`

type Org struct {
	ID          string
	Name        string
	DisplayName string
	Members     struct {
		Nodes []User
	}
}
