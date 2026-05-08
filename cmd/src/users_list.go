package main

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const usersListExamples = `
Examples:

  List users:

    	$ src users list

  List users whose names match the query:

    	$ src users list -query='myquery'

  List all users with the "foo" tag:

    	$ src users list -tag=foo

`

var usersListCommand = clicompat.Wrap(&cli.Command{
	Name:        "list",
	Usage:       "lists users",
	UsageText:   "src users list [options]",
	Description: usersListExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.IntFlag{
			Name:  "first",
			Value: 1000,
			Usage: "Returns the first n users from the list.",
		},
		&cli.StringFlag{
			Name:  "query",
			Usage: `Returns users whose names match the query. (e.g. "alice")`,
		},
		&cli.StringFlag{
			Name:  "tag",
			Usage: `Returns users with the given tag.`,
		},
		&cli.StringFlag{
			Name:  "f",
			Value: "{{.Username}}",
			Usage: `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Username}} ({{.DisplayName}})" or "{{.|json}}")`,
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		first := cmd.Int("first")
		queryValue := cmd.String("query")
		tag := cmd.String("tag")
		format := cmd.String("f")

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		tmpl, err := parseTemplate(format)
		if err != nil {
			return err
		}
		vars := map[string]any{
			"first": api.NullInt(first),
			"query": api.NullString(queryValue),
			"tag":   api.NullString(tag),
		}
		queryTagVar := ""
		queryTag := ""
		if maybeTagVar, ok := vars["tag"].(*string); ok && maybeTagVar != nil {
			queryTagVar = `$tag: String,`
			queryTag = `tag: $tag,`
		}
		query := `query Users(
  $first: Int,
  $query: String,
` + queryTagVar + `
) {
  users(
first: $first,
    query: $query,
` + queryTag + `
  ) {
    nodes {
      ...UserFields
    }
  }
}` + userFragment

		var result struct {
			Users struct {
				Nodes []User
			}
		}
		if ok, err := client.NewRequest(query, vars).Do(ctx, &result); err != nil || !ok {
			return err
		}

		for _, user := range result.Users.Nodes {
			if err := execTemplate(tmpl, user); err != nil {
				return err
			}
		}
		return nil
	},
})
