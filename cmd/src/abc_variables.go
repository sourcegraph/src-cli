package main

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

var abcVariablesCommand = clicompat.WithLegacyHelp(&cli.Command{
	Name:  "variables",
	Usage: "manage workflow instance variables",
	UsageText: `'src abc variables' is a tool that manages workflow instance variables on agentic batch changes.

Usage:

	src abc variables command [command options]

The commands are:`,
	Description:     `Use "src abc variables [command] -h" for more information about a command.`,
	HideHelpCommand: true,
	HideVersion:     true,
	Commands: []*cli.Command{
		abcVariablesSetCommand,
		abcVariablesDeleteCommand,
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		cli.HelpPrinter(c.Root().Writer, c.CustomRootCommandHelpTemplate, c)
		return nil
	},
})
