package main

import (
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

var contextCommand = clicompat.Wrap(&cli.Command{
	Name:        "context",
	Usage:       "manages search contexts",
	UsageText:   "src context [command options]",
	Description: contextExamples,
	HideVersion: true,
	Commands: []*cli.Command{
		contextImportCommand,
	},
})

const contextExamples = `'src context' is a tool that manages search contexts on a Sourcegraph instance.

Usage:

	src context command [command options]

The commands are:

	import     creates a search context from an SBOM

Use "src context [command] -h" for more information about a command.
`
