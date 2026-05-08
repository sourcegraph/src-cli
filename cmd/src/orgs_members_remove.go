package main

import (
	"context"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const orgsMembersRemoveExamples = `
Examples:

  Remove a member (alice) from an organization (abc-org):

    	$ src orgs members remove -org-id=$(src org get -f '{{.ID}}' -name=abc-org) -user-id=$(src users get -f '{{.ID}}' -username=alice)
`

var orgsMembersRemoveCommand = clicompat.Wrap(&cli.Command{
	Name:        "remove",
	Usage:       "removes a user as a member from an organization",
	UsageText:   "src orgs members remove [options]",
	Description: orgsMembersRemoveExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "org-id",
			Usage: "ID of organization from which to remove member. (required)",
		},
		&cli.StringFlag{
			Name:  "user-id",
			Usage: "ID of user to remove as member. (required)",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		orgID := cmd.String("org-id")
		userID := cmd.String("user-id")

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		query := `mutation RemoveUserFromOrg(
  $orgID: ID!,
  $userID: ID!,
) {
  removeUserFromOrg(
    orgID: $orgID,
    userID: $userID,
  ) {
    alwaysNil
  }
}`

		var result struct {
			RemoveUserFromOrg struct{}
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"orgID":  orgID,
			"userID": userID,
		}).Do(ctx, &result); err != nil || !ok {
			return err
		}

		_, err := fmt.Fprintf(cmd.Writer, "User %q removed as member from organization with ID %q.\n", userID, orgID)
		return err
	},
})
