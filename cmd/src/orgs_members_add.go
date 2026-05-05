package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
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

func init() {
	usage := orgsMembersAddExamples

	flagSet := flag.NewFlagSet("add", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src orgs members %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		orgIDFlag    = flagSet.String("org-id", "", "ID of organization to which to add member. (required)")
		usernameFlag = flagSet.String("username", "", "Username of user to add as member. (required)")
		apiFlags     = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

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
			"organization": *orgIDFlag,
			"username":     *usernameFlag,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		fmt.Printf("User %q added as member to organization with ID %q.\n", *usernameFlag, *orgIDFlag)
		return nil
	}

	// Register the command.
	orgsMembersCommands = append(orgsMembersCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
