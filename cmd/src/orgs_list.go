package main

import (
	"context"
	"flag"
	"fmt"

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

func init() {
	usage := orgsListExamples

	flagSet := flag.NewFlagSet("list", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src orgs %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		firstFlag  = flagSet.Int("first", 1000, "Returns the first n organizations from the list.")
		queryFlag  = flagSet.String("query", "", `Returns organizations whose names match the query. (e.g. "alice")`)
		formatFlag = flagSet.String("f", "{{.Name}}", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Name}} ({{.DisplayName}})" or "{{.|json}}")`)
		apiFlags   = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		tmpl, err := parseTemplate(*formatFlag)
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
			"first": api.NullInt(*firstFlag),
			"query": api.NullString(*queryFlag),
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		for _, org := range result.Organizations.Nodes {
			if err := execTemplate(tmpl, org); err != nil {
				return err
			}
		}
		return nil
	}

	// Register the command.
	orgsCommands = append(orgsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
