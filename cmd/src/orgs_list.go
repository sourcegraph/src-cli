package main

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const orgsListExamples = `
Examples:

  List organizations:

    	$ src orgs list

  List organizations whose names match the query:

    	$ src orgs list -query='myquery'

`

var orgsListCommand = clicompat.Wrap(&cli.Command{
	Name:        "list",
	Usage:       "lists organizations",
	UsageText:   "src orgs list [options]",
	Description: orgsListExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.IntFlag{
			Name:  "first",
			Value: 1000,
			Usage: "Returns the first n organizations from the list.",
		},
		&cli.StringFlag{
			Name:  "query",
			Usage: `Returns organizations whose names match the query. (e.g. "alice")`,
		},
		&cli.StringFlag{
			Name:  "f",
			Value: "{{.Name}}",
			Usage: `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Name}} ({{.DisplayName}})" or "{{.|json}}")`,
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		first := cmd.Int("first")
		queryValue := cmd.String("query")
		format := cmd.String("f")

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		tmpl, err := parseTemplate(format)
		if err != nil {
			return err
		}

		query := `query Organizations(
  $first: Int,
  $query: String,
) {
  organizations(
    first: $first,
    query: $query,
  ) {
    nodes {
      ...OrgFields
    }
  }
}` + orgFragment

		var result struct {
			Organizations struct {
				Nodes []Org
			}
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"first": api.NullInt(first),
			"query": api.NullString(queryValue),
		}).Do(ctx, &result); err != nil || !ok {
			return err
		}

		for _, org := range result.Organizations.Nodes {
			if err := execTemplate(tmpl, org); err != nil {
				return err
			}
		}
		return nil
	},
})
