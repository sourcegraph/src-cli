package main

import (
	"context"
	"strings"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/deepsearch"
	"github.com/urfave/cli/v3"
)

const deepsearchGetExamples = `
Examples:

  Get a conversation by resource name:

    	$ src deepsearch get users/-/conversations/abc123

`

var deepsearchGetCommand = clicompat.Wrap(&cli.Command{
	Name:        "get",
	Usage:       "gets a Deep Search conversation",
	UsageText:   "src deepsearch get [options] <conversation-name>",
	Description: deepsearchGetExamples,
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

		conversation, ok, err := cfg.deepsearchClient(cmd).GetConversation(ctx, deepsearch.GetConversationRequest{Name: name})
		if err != nil || !ok {
			return err
		}
		return execTemplate(tmpl, conversation)
	},
})

func deepsearchName(cmd *cli.Command) (string, error) {
	if !cmd.Args().Present() {
		return "", cmderrors.Usage("must provide a conversation name")
	}

	name := strings.TrimSpace(cmd.Args().First())
	if name == "" {
		return "", cmderrors.Usage("must provide a conversation name")
	}
	if cmd.Args().Len() > 1 {
		return "", cmderrors.Usage("expected exactly one conversation name")
	}
	return name, nil
}
