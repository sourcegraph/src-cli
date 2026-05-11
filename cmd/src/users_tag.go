package main

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const usersTagExamples = `
Examples:

  Add a tag "foo" to a user:

    	$ src users tag -user-id=$(src users get -f '{{.ID}}' -username=alice) -tag=foo

  Remove a tag "foo" to a user:

    	$ src users tag -user-id=$(src users get -f '{{.ID}}' -username=alice) -remove -tag=foo

Related examples:

  List all users with the "foo" tag:

    	$ src users list -tag=foo

`

var usersTagCommand = clicompat.Wrap(&cli.Command{
	Name:        "tag",
	Usage:       "add/remove a tag on a user",
	UsageText:   "src users tag [options]",
	Description: usersTagExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:      "user-id",
			Usage:     "The ID of the user to tag.",
			Required:  true,
			Validator: requiresNotEmpty("provide a user ID by using -user-id"),
		},
		&cli.StringFlag{
			Name:      "tag",
			Usage:     "The tag to set on the user.",
			Required:  true,
			Validator: requiresNotEmpty("provide a tag by using -tag"),
		},
		&cli.BoolFlag{
			Name:  "remove",
			Usage: "Remove the tag. (default: add the tag)",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		userID := cmd.String("user-id")
		tag := cmd.String("tag")
		remove := cmd.Bool("remove")

		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		query := `mutation SetUserTag(
  $user: ID!,
  $tag: String!,
  $present: Boolean!
) {
  setTag(
    node: $user,
    tag: $tag,
    present: $present
  ) {
    alwaysNil
  }
}`

		_, err := client.NewRequest(query, map[string]any{
			"user":    userID,
			"tag":     tag,
			"present": !remove,
		}).Do(ctx, &struct{}{})
		return err
	},
})
