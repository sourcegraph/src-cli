package main

import (
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
		}),
	},
})
