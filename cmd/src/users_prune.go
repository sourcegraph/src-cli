package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/urfave/cli/v3"
)

const usersPruneExamples = `
This command removes users from a Sourcegraph instance who have been inactive for 60 or more days. Admin accounts are omitted by default.
	
Examples:

	$ src users prune -days 182
	
	$ src users prune -remove-admin -remove-null-users
`

var usersPruneCommand = clicompat.Wrap(&cli.Command{
	Name:        "prune",
	Usage:       "deletes inactive users",
	UsageText:   "src users prune [options]",
	Description: usersPruneExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.IntFlag{
			Name:  "days",
			Value: 60,
			Usage: "Days threshold on which to remove users, must be 60 days or greater and defaults to this value ",
		},
		&cli.BoolFlag{
			Name:  "remove-admin",
			Usage: "prune admin accounts",
		},
		&cli.BoolFlag{
			Name:  "remove-null-users",
			Usage: "removes users with no last active value",
		},
		&cli.BoolFlag{
			Name:  "force",
			Usage: "skips user confirmation step allowing programmatic use",
		},
		&cli.BoolFlag{
			Name:  "display-users",
			Usage: "display table of users to be deleted by prune",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		daysToDelete := cmd.Int("days")
		removeAdmin := cmd.Bool("remove-admin")
		removeNoLastActive := cmd.Bool("remove-null-users")
		skipConfirmation := cmd.Bool("force")
		displayUsersToDelete := cmd.Bool("display-users")

		if daysToDelete < 60 {
			_, err := fmt.Fprintln(cmd.Writer, "-days flag must be set to 60 or greater")
			return err
		}

		apiFlags := clicompat.APIFlagsFromCmd(cmd)
		client := cfg.apiClient(apiFlags, cmd.Writer)

		// get current user so as not to delete issuer of the prune request
		currentUserQuery := `query getCurrentUser { currentUser { username }}`
		var currentUserResult struct {
			CurrentUser struct {
				Username string
			}
		}
		if ok, err := client.NewRequest(currentUserQuery, nil).Do(ctx, &currentUserResult); err != nil || !ok {
			return err
		}

		// get total users to paginate over
		totalUsersQuery := `query getTotalUsers { site { users { totalCount }}}`
		var totalUsers struct {
			Site struct {
				Users struct {
					TotalCount float64
				}
			}
		}
		if ok, err := client.NewRequest(totalUsersQuery, nil).Do(ctx, &totalUsers); err != nil || !ok {
			return err
		}

		// get 100 site users
		getInactiveUsersQuery := `
		query getInactiveUsers($limit: Int $offset: Int) {
	site {
		users {
			nodes (limit: $limit offset: $offset) {
				id
				username
				email
				siteAdmin
				lastActiveAt
				deletedAt
			}
		}
	}
}
`

		// paginate through users
		var aggregatedUsers []SiteUser
		// pagination variables, limit set to maximum possible users returned per request
		offset := 0
		const limit int = 100

		// paginate requests until all site users have been checked -- this includes soft deleted users
		for len(aggregatedUsers) < int(totalUsers.Site.Users.TotalCount) {
			pagVars := map[string]any{
				"offset": offset,
				"limit":  limit,
			}

			var usersResult struct {
				Site struct {
					Users struct {
						Nodes []SiteUser
					}
					TotalCount float64
				}
			}
			if ok, err := client.NewRequest(getInactiveUsersQuery, pagVars).Do(ctx, &usersResult); err != nil || !ok {
				return err
			}
			// increment graphql request offset by the length of the last user set returned
			offset = offset + len(usersResult.Site.Users.Nodes)
			// append graphql user results to aggregated users to be processed against user removal conditions
			aggregatedUsers = append(aggregatedUsers, usersResult.Site.Users.Nodes...)
		}

		// filter users for deletion
		usersToDelete := make([]UserToDelete, 0)
		for _, user := range aggregatedUsers {
			// never remove user issuing command
			if user.Username == currentUserResult.CurrentUser.Username {
				continue
			}
			// filter out soft deleted users returned by site graphql endpoint
			if user.DeletedAt != "" {
				continue
			}
			//compute days since last use
			daysSinceLastUse, hasLastActive, err := computeDaysSinceLastUse(user)
			if err != nil {
				return err
			}
			// don't remove users with no last active value unless option flag is set
			if !hasLastActive && !removeNoLastActive {
				continue
			}
			// don't remove admins unless option flag is set
			if !removeAdmin && user.SiteAdmin {
				continue
			}
			// remove users who have been inactive for longer than the threshold set by the -days flag
			if daysSinceLastUse <= daysToDelete && hasLastActive {
				continue
			}
			// serialize user to print in table as part of confirmUserRemoval, add to delete slice
			userToDelete := UserToDelete{user, daysSinceLastUse}
			usersToDelete = append(usersToDelete, userToDelete)
		}

		if skipConfirmation {
			for _, user := range usersToDelete {
				if err := removeUser(user.User, client, ctx); err != nil {
					return err
				}
			}
			return nil
		}

		// confirm and remove users
		if confirmed, _ := confirmUserRemoval(usersToDelete, daysToDelete, displayUsersToDelete); !confirmed {
			fmt.Fprintln(cmd.Writer, "Aborting removal")
			return nil
		} else {
			fmt.Fprintln(cmd.Writer, "REMOVING USERS")
			for _, user := range usersToDelete {
				if err := removeUser(user.User, client, ctx); err != nil {
					return err
				}
			}
		}

		return nil
	},
})

// computes days since last usage from current day and time and aggregated_user_statistics.lastActiveAt, uses time.Parse
func computeDaysSinceLastUse(user SiteUser) (timeDiff int, hasLastActive bool, _ error) {
	// handle for null LastActiveAt, users who have never been active
	if user.LastActiveAt == "" {
		hasLastActive = false
		return 0, hasLastActive, nil
	}
	timeLast, err := time.Parse(time.RFC3339, user.LastActiveAt)
	if err != nil {
		return 0, false, err
	}
	timeDiff = int(time.Since(timeLast).Hours() / 24)

	return timeDiff, true, err
}

// Issue graphQL api request to remove user
func removeUser(user SiteUser, client api.Client, ctx context.Context) error {
	query := `mutation DeleteUser($user: ID!) { deleteUser(user: $user) { alwaysNil }}`
	vars := map[string]any{
		"user": user.ID,
	}
	if ok, err := client.NewRequest(query, vars).Do(ctx, nil); err != nil || !ok {
		return err
	}
	return nil
}

type UserToDelete struct {
	User             SiteUser
	DaysSinceLastUse int
}

// Verify user wants to remove users with table of users and a command prompt for [y/N]
func confirmUserRemoval(usersToDelete []UserToDelete, daysThreshold int, displayUsers bool) (bool, error) {
	if displayUsers {
		fmt.Printf("Users to remove from %s\n", cfg.endpointURL)
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendHeader(table.Row{"Username", "Email", "Days Since Last Active"})
		for _, user := range usersToDelete {
			if user.User.Email != "" {
				t.AppendRow([]any{user.User.Username, user.User.Email, user.DaysSinceLastUse})
				t.AppendSeparator()
			} else {
				t.AppendRow([]any{user.User.Username, "", user.DaysSinceLastUse})
				t.AppendSeparator()
			}
		}
		t.SetStyle(table.StyleRounded)
		t.Render()
	}
	input := ""
	for strings.ToLower(input) != "y" && strings.ToLower(input) != "n" {
		fmt.Printf("%v users were inactive for more than %v days on %v.\nDo you  wish to proceed with user removal [y/N]: ", len(usersToDelete), daysThreshold, cfg.endpointURL)
		if _, err := fmt.Scanln(&input); err != nil {
			return false, err
		}
	}
	return strings.ToLower(input) == "y", nil
}
