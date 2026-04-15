package main

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

var abcVariablesCommand = &cli.Command{
	Name:  "variables",
	Usage: "manage workflow instance variables",
	UsageText: `'src abc variables' is a tool that manages workflow instance variables on agentic batch changes.

Usage:

	src abc variables command [command options]

The commands are:`,
	OnUsageError:    clicompat.OnUsageError,
	Description:     `Use "src abc variables [command] -h" for more information about a command.`,
	HideHelpCommand: true,
	HideVersion:     true,
	Commands: []*cli.Command{
		abcVariablesSetCommand,
		abcVariablesDeleteCommand,
	},
	Action: func(_ context.Context, c *cli.Command) error {
		return cli.ShowSubcommandHelp(c)
	},
}
