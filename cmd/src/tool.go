package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

var toolCmd = &cli.Command{
	Name:        "src tool",
	Usage:       "Exposes tools for AI agents to interact with Sourcegraph (EXPERIMENTAL)",
	Description: "The tool subcommand exposes tools that can be used by AI agents to perform tasks against Sourcegraph instances.",
	Commands:    []*cli.Command{},
	Writer:      os.Stdout,
	Action: func(ctx context.Context, c *cli.Command) error {
		fmt.Println("Not implemented")
		return nil
	},
}
