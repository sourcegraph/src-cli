package main

import (
	"context"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const orgsDeleteExamples = `
Examples:

  Delete an organization by ID:

    	$ src orgs delete -id=VXNlcjox

  Delete an organization by name:

    	$ src orgs delete -id=$(src orgs get -f='{{.ID}}' -name=abc-org)

  Delete all organizations that match the query

    	$ src orgs list -f='{{.ID}}' -query=abc-org | xargs -n 1 -I ORGID src orgs delete -id=ORGID

`

var orgsDeleteCommand = clicompat.Wrap(&cli.Command{
	Name:        "delete",
	Usage:       "deletes an organization",
	UsageText:   "src orgs delete [options]",
	Description: orgsDeleteExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "id",
			Usage: "The ID of the organization to delete.",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		orgID := cmd.String("id")
		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		query := `mutation DeleteOrganization(
  $organization: ID!
) {
  deleteOrganization(
    organization: $organization
  ) {
    alwaysNil
  }
}`

		var result struct {
			DeleteOrganization struct{}
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"organization": orgID,
		}).Do(ctx, &result); err != nil || !ok {
			return err
		}

		_, err := fmt.Fprintf(cmd.Writer, "Organization with ID %q deleted.\n", orgID)
		return err
	},
})
