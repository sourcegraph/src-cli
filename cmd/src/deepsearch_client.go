package main

import (
	"github.com/sourcegraph/src-cli/internal/api/connect"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/deepsearch"
	"github.com/urfave/cli/v3"
)

func (c *config) connectClient(cmd *cli.Command) connect.Client {
	flags := clicompat.APIFlagsFromCmd(cmd)
	return connect.NewClient(c.apiClient(flags, cmd.Writer))
}

func (c *config) deepsearchClient(cmd *cli.Command) *deepsearch.Client {
	return deepsearch.NewClient(c.connectClient(cmd))
}
