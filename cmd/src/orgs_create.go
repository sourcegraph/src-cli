package main

import (
	"context"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const orgsCreateExamples = `
Examples:

  Create an organization:

    	$ src orgs create -name=abc-org -display-name='ABC Organization'

`

var orgsCreateCommand = clicompat.Wrap(&cli.Command{
	Name:        "create",
	Usage:       "creates an organization",
	UsageText:   "src orgs create [options]",
	Description: orgsCreateExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "name",
			Usage: "The new organization's name. (required)",
		},
		&cli.StringFlag{
			Name:  "display-name",
			Usage: "The new organization's display name. Defaults to organization name if unspecified.",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		name := cmd.String("name")
		displayName := cmd.String("display-name")

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		query := `mutation CreateOrg(
  $name: String!,
  $displayName: String!,
) {
  createOrganization(
    name: $name,
    displayName: $displayName,
  ) {
    id
  }
}`

		var result struct {
			CreateOrg Org
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"name":        name,
			"displayName": displayName,
		}).Do(ctx, &result); err != nil || !ok {
			return err
		}

		_, err := fmt.Fprintf(cmd.Writer, "Organization %q created.\n", name)
		return err
	},
})
