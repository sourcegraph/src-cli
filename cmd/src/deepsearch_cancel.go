package main

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/deepsearch"
	"github.com/urfave/cli/v3"
)

const deepsearchCancelExamples = `
Examples:

  Cancel a conversation:

    	$ src deepsearch cancel users/-/conversations/abc123

`

var deepsearchCancelCommand = clicompat.Wrap(&cli.Command{
	Name:        "cancel",
	Usage:       "cancels an in-progress Deep Search conversation",
	UsageText:   "src deepsearch cancel [options] <conversation-name>",
	Description: deepsearchCancelExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "f",
			Value: "{{.|json}}",
			Usage: `Format for the output, using the syntax of Go package text/template.`,
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		name, err := deepsearchName(cmd)
		if err != nil {
			return err
		}
		tmpl, err := parseTemplate(cmd.String("f"))
		if err != nil {
			return err
		}

		conversation, ok, err := cfg.deepsearchClient(cmd).CancelConversation(ctx, deepsearch.CancelConversationRequest{Name: name})
		if err != nil || !ok {
			return err
		}
		return execTemplate(tmpl, conversation)
	},
})
