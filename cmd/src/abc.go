package main

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

var abcCommand = clicompat.Wrap(&cli.Command{
	Name:  "abc",
	Usage: "manages agentic batch changes",
	Commands: []*cli.Command{
		clicompat.Wrap(&cli.Command{
			Name:  "variables",
			Usage: "manage workflow instance variables",
			Commands: []*cli.Command{
				abcVariablesSetCommand,
				abcVariablesDeleteCommand,
			},
			Action: func(ctx context.Context, cmd *cli.Command) error {
				return cli.ShowSubcommandHelp(cmd)
			},
		}),
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		return cli.ShowSubcommandHelp(cmd)
	},
})
