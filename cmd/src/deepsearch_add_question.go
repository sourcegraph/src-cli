package main

import (
	"context"
	"strings"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/deepsearch"
	"github.com/urfave/cli/v3"
)

const deepsearchAddQuestionExamples = `
Examples:

  Ask a follow-up question in an existing conversation:

    	$ src deepsearch add-question users/-/conversations/abc123 'What calls this code?'

`

var deepsearchAddQuestionCommand = clicompat.Wrap(&cli.Command{
	Name:        "add-question",
	Usage:       "adds a follow-up question to a Deep Search conversation",
	UsageText:   "src deepsearch add-question [options] <conversation-name> <question>",
	Description: deepsearchAddQuestionExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "f",
			Value: "{{.|json}}",
			Usage: `Format for the output, using the syntax of Go package text/template.`,
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		parent, question, err := deepsearchParentAndQuestion(cmd)
		if err != nil {
			return err
		}

		tmpl, err := parseTemplate(cmd.String("f"))
		if err != nil {
			return err
		}

		response, ok, err := cfg.deepsearchClient(cmd).AddConversationQuestion(ctx, deepsearch.AddConversationQuestionRequest{
			Parent:   parent,
			Question: deepsearch.NewQuestion(question),
		})
		if err != nil || !ok {
			return err
		}
		return execTemplate(tmpl, response)
	},
})

func deepsearchParentAndQuestion(cmd *cli.Command) (string, string, error) {
	args := cmd.Args().Slice()
	if len(args) == 0 {
		return "", "", cmderrors.Usage("must provide a conversation name")
	}

	parent := strings.TrimSpace(args[0])
	if parent == "" {
		return "", "", cmderrors.Usage("must provide a conversation name")
	}

	question := strings.TrimSpace(strings.Join(args[1:], " "))
	if question == "" {
		return "", "", cmderrors.Usage("must provide a question")
	}
	return parent, question, nil
}
