package main

import (
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

var usersCommand = clicompat.Wrap(&cli.Command{
	Name:        "users",
	Aliases:     []string{"user"},
	Usage:       "manages users",
	UsageText:   "src users [command options]",
	Description: usersExamples,
	HideVersion: true,
	Commands: []*cli.Command{
		usersListCommand,
		usersGetCommand,
		usersCreateCommand,
		usersDeleteCommand,
		usersPruneCommand,
		usersTagCommand,
	},
})

const usersExamples = `'src users' is a tool that manages users on a Sourcegraph instance.

Usage:

	src users command [command options]

The commands are:

	list       lists users
	get        gets a user
	create     creates a user account
	delete     deletes a user account
	prune      deletes inactive users
	tag        add/remove a tag on a user

Use "src users [command] -h" for more information about a command.
`

const userFragment = `
fragment UserFields on User {
    id
    username
    displayName
    siteAdmin
    organizations {
		nodes {
        	id
        	name
        	displayName
		}
    }
    emails {
        email
		verified
    }
    url
}
`

type User struct {
	ID            string
	Username      string
	DisplayName   string
	SiteAdmin     bool
	Organizations struct {
		Nodes []Org
	}
	Emails []UserEmail
	URL    string
}

type UserEmail struct {
	Email    string
	Verified bool
}

type SiteUser struct {
	ID           string
	Username     string
	Email        string
	SiteAdmin    bool
	LastActiveAt string
	DeletedAt    string
}
