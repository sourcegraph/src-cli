package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
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

func init() {
	usage := orgsMembersRemoveExamples

	flagSet := flag.NewFlagSet("remove", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src orgs members %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		orgIDFlag  = flagSet.String("org-id", "", "ID of organization from which to remove member. (required)")
		userIDFlag = flagSet.String("user-id", "", "ID of user to remove as member. (required)")
		apiFlags   = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

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
			"orgID":  *orgIDFlag,
			"userID": *userIDFlag,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		fmt.Printf("User %q removed as member from organization with ID %q.\n", *userIDFlag, *orgIDFlag)
		return nil
	}

	// Register the command.
	orgsMembersCommands = append(orgsMembersCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
