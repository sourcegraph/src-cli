package main

import (
	"context"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/deepsearch"
	"github.com/urfave/cli/v3"
)

const deepsearchDeleteExamples = `
Examples:

  Permanently delete a conversation:

    	$ src deepsearch delete users/-/conversations/abc123

`

var deepsearchDeleteCommand = clicompat.Wrap(&cli.Command{
	Name:        "delete",
	Usage:       "permanently deletes a Deep Search conversation",
	UsageText:   "src deepsearch delete [options] <conversation-name>",
	Description: deepsearchDeleteExamples,
	HideVersion: true,
	Flags:       clicompat.WithAPIFlags(),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		name, err := deepsearchName(cmd)
		if err != nil {
			return err
		}

		ok, err := cfg.deepsearchClient(cmd).DeleteConversation(ctx, deepsearch.DeleteConversationRequest{Name: name})
		if err != nil || !ok {
			return err
		}
		_, err = fmt.Fprintf(cmd.Writer, "Deep Search conversation %q deleted.\n", name)
		return err
	},
})
