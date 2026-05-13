package main

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/deepsearch"
	"github.com/urfave/cli/v3"
)

const deepsearchListExamples = `
Examples:

  List recent Deep Search conversation summaries:

    	$ src deepsearch list

  List summaries whose content matches a query:

    	$ src deepsearch list -query='auth'

`

var deepsearchListCommand = clicompat.Wrap(&cli.Command{
	Name:        "list",
	Usage:       "lists Deep Search conversation summaries",
	UsageText:   "src deepsearch list [options]",
	Description: deepsearchListExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "parent",
			Usage: `Parent resource. Defaults to the authenticated user. (e.g. "users/-")`,
		},
		&cli.IntFlag{
			Name:  "page-size",
			Value: 100,
			Usage: "Maximum number of conversations to return.",
		},
		&cli.StringFlag{
			Name:  "page-token",
			Usage: "Page token from a previous response.",
		},
		&cli.StringFlag{
			Name:  "query",
			Usage: "Return conversations whose content matches the query.",
		},
		&cli.BoolFlag{
			Name:  "starred",
			Usage: "Return only starred conversations. Use --starred=false for unstarred conversations.",
		},
		&cli.BoolFlag{
			Name:  "all",
			Usage: "Fetch all pages.",
		},
		&cli.StringFlag{
			Name:  "f",
			Value: "{{.|json}}",
			Usage: `Format for the output, using the syntax of Go package text/template.`,
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Len() > 0 {
			return cmderrors.Usage("additional arguments not allowed")
		}
		if cmd.Int("page-size") < 0 {
			return cmderrors.Usage("page-size must be greater than or equal to 0")
		}

		tmpl, err := parseTemplate(cmd.String("f"))
		if err != nil {
			return err
		}

		request := deepsearch.ListConversationSummariesRequest{
			Parent:    cmd.String("parent"),
			PageSize:  cmd.Int("page-size"),
			PageToken: cmd.String("page-token"),
		}
		if query := cmd.String("query"); query != "" {
			request.Filters = append(request.Filters, deepsearch.ListConversationSummariesFilter{ContentQuery: query})
		}
		if cmd.IsSet("starred") {
			starred := cmd.Bool("starred")
			request.Filters = append(request.Filters, deepsearch.ListConversationSummariesFilter{Starred: &starred})
		}

		client := cfg.deepsearchClient(cmd)
		var summaries []deepsearch.ConversationSummary
		for {
			response, ok, err := client.ListConversationSummaries(ctx, request)
			if err != nil || !ok {
				return err
			}
			if !cmd.Bool("all") {
				return execTemplate(tmpl, response)
			}
			summaries = append(summaries, response.ConversationSummaries...)
			if response.NextPageToken == "" {
				return execTemplate(tmpl, deepsearch.ListConversationSummariesResponse{ConversationSummaries: summaries})
			}
			request.PageToken = response.NextPageToken
		}
	},
})
