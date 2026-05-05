package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
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

func init() {
	usage := orgsDeleteExamples

	flagSet := flag.NewFlagSet("delete", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src orgs %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		orgIDFlag = flagSet.String("id", "", `The ID of the organization to delete.`)
		apiFlags  = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

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
			"organization": *orgIDFlag,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		fmt.Printf("Organization with ID %q deleted.\n", *orgIDFlag)
		return nil
	}

	// Register the command.
	orgsCommands = append(orgsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
