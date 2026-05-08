package main

import (
	"context"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const orgsMembersAddExamples = `
Examples:

  Add a member (alice) to an organization (abc-org):

    	$ src orgs members add -org-id=$(src org get -f '{{.ID}}' -name=abc-org) -username=alice

`

var orgsMembersAddCommand = clicompat.Wrap(&cli.Command{
	Name:        "add",
	Usage:       "adds a user as a member to an organization",
	UsageText:   "src orgs members add [options]",
	Description: orgsMembersAddExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "org-id",
			Usage: "ID of organization to which to add member. (required)",
		},
		&cli.StringFlag{
			Name:  "username",
			Usage: "Username of user to add as member. (required)",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		orgID := cmd.String("org-id")
		username := cmd.String("username")

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		query := `mutation AddUserToOrganization(
  $organization: ID!,
  $username: String!,
) {
  addUserToOrganization(
    organization: $organization,
    username: $username,
  ) {
    alwaysNil
  }
}`

		var result struct {
			AddUserToOrganization struct{}
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"organization": orgID,
			"username":     username,
		}).Do(ctx, &result); err != nil || !ok {
			return err
		}

		_, err := fmt.Fprintf(cmd.Writer, "User %q added as member to organization with ID %q.\n", username, orgID)
		return err
	},
})
