package main

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const orgsGetExamples = `
Examples:

  Get organization named abc-org:

    	$ src orgs get -name=abc-org

  List usernames of members of organization named abc-org (replace '.Username' with '.ID' to list user IDs):

    	$ src orgs get -f '{{range $i,$ := .Members.Nodes}}{{if ne $i 0}}{{"\n"}}{{end}}{{.Username}}{{end}}' -name=abc-org

`

var orgsGetCommand = clicompat.Wrap(&cli.Command{
	Name:        "get",
	Usage:       "gets an organization",
	UsageText:   "src orgs get [options]",
	Description: orgsGetExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "name",
			Usage: `Look up organization by name. (e.g. "abc-org")`,
		},
		&cli.StringFlag{
			Name:  "f",
			Value: "{{.|json}}",
			Usage: `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Name}} ({{.DisplayName}})")`,
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		name := cmd.String("name")
		format := cmd.String("f")

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		tmpl, err := parseTemplate(format)
		if err != nil {
			return err
		}

		query := `query Organization(
  $name: String!,
) {
  organization(
    name: $name
  ) {
    ...OrgFields
  }
}` + orgFragment

		var result struct {
			Organization *Org
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"name": name,
		}).Do(ctx, &result); err != nil || !ok {
			return err
		}

		return execTemplate(tmpl, result.Organization)
	},
})
