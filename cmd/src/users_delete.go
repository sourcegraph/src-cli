package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const usersDeleteExamples = `
Examples:

  Delete a user account by ID:

    	$ src users delete -id=VXNlcjox

  Delete a user account by username:

    	$ src users delete -id=$(src users get -f='{{.ID}}' -username=alice)

  Delete all user accounts that match the query:

    	$ src users list -f='{{.ID}}' -query=alice | xargs -n 1 -I USERID src users delete -id=USERID

`

var usersDeleteCommand = clicompat.Wrap(&cli.Command{
	Name:        "delete",
	Usage:       "deletes a user account",
	UsageText:   "src users delete [options]",
	Description: usersDeleteExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "id",
			Usage: "The ID of the user to delete.",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		userID := cmd.String("id")
		client := cfg.apiClient(clicompat.APIFlagsFromCmd(cmd), cmd.Writer)

		if userID == "" {
			query := `query UsersTotalCountCountUsers { users { totalCount } }`

			var result struct {
				Users struct {
					TotalCount int
				}
			}
			ok, err := client.NewQuery(query).Do(ctx, &result)
			if err != nil || !ok {
				return err
			}

			if _, err := fmt.Fprintf(cmd.Writer, "No user ID specified. This would delete %d users.\nType in this number to confirm and hit return: ", result.Users.TotalCount); err != nil {
				return err
			}
			reader := bufio.NewReader(os.Stdin)
			text, err := reader.ReadString('\n')
			if err != nil {
				return err
			}

			count, err := strconv.Atoi(strings.TrimSpace(text))
			if err != nil {
				return err
			}

			if count != result.Users.TotalCount {
				_, err := fmt.Fprintln(cmd.Writer, "Number does not match. Aborting.")
				return err
			}
		}

		query := `mutation DeleteUser(
  $user: ID!
) {
  deleteUser(
    user: $user
  ) {
    alwaysNil
  }
}`

		var result struct {
			DeleteUser struct{}
		}
		if ok, err := client.NewRequest(query, map[string]any{
			"user": userID,
		}).Do(ctx, &result); err != nil || !ok {
			return err
		}

		_, err := fmt.Fprintf(cmd.Writer, "User with ID %q deleted.\n", userID)
		return err
	},
})
