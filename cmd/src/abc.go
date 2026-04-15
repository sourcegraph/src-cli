package main

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

var abcCommand = &cli.Command{
	Name:  "abc",
	Usage: "manages agentic batch changes",
	UsageText: `'src abc' is a tool that manages agentic batch changes.

Usage:

	src abc command [command options]

The commands are:`,
	OnUsageError:    clicompat.OnUsageError,
	Description:     `Use "src abc [command] -h" for more information about a command.`,
	HideHelpCommand: true,
	HideVersion:     true,
	Commands: []*cli.Command{
		abcVariablesCommand,
	},
	Action: func(_ context.Context, c *cli.Command) error {
		return cli.ShowSubcommandHelp(c)
	},
}
