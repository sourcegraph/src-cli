package main

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const authExamples = `
Authentication-related helper commands.

Examples:

Print the active auth token:

$ src auth token
sgp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

Print the current Authorization header:

$ src auth token --header
Authorization: token sgp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
`

var authCommand = clicompat.Wrap(&cli.Command{
	Name:        "auth",
	Usage:       "authentication helper commands",
	UsageText:   "src auth [command options]",
	Description: authExamples,
	HideVersion: true,
	Commands: []*cli.Command{
		authTokenCommand,
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		return cli.ShowSubcommandHelp(cmd)
	},
})
