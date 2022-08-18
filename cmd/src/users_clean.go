package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"
)

func init() {
	usage := `
Examples:

	$ src users clean -d 182

`

	flagSet := flag.NewFlagSet("clean", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src users %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		daysToDelete       = flagSet.Int("d", 365, "Day threshold on which to remove users, defaults to 365")
		noAdmin            = flagSet.Bool("no-admin", false, "Omit admin accounts from cleanup")
		toEmail            = flagSet.Bool("email", false, "send removed users an email")
		removeNoLastActive = flagSet.Bool("removeNeverActive", false, "removes users with null lastActive value")
		apiFlags           = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		flagSet.Parse(args)

		ctx := context.Background()
		client := cfg.apiClient(apiFlags, flagSet.Output())

		vars := map[string]interface{}{
			"-d": api.NullInt(*daysToDelete),
		}

		query := `
query Users($first: Int, $query: String) {
	users(first: $first, query: $query) {
		nodes {
			...UserFields
		}
	}
}
` + userFragment

		// get users to delete
		var result struct {
			Users struct {
				Nodes []User
			}
		}
		if ok, err := client.NewRequest(query, vars).Do(ctx, &result); err != nil || !ok {
			return err
		}

		usersToDelete := make([]UserToDelete, 0)
		for _, user := range result.Users.Nodes {
			daysSinceLastUse, wasLastActive, err := computeDaysSinceLastUse(user)
			if err != nil {
				return err
			}
			if !wasLastActive && !*removeNoLastActive {
				continue
			}
			if *noAdmin && user.SiteAdmin {
				continue
			}
			if daysSinceLastUse <= *daysToDelete && wasLastActive {
				continue
			}
			deleteUser := UserToDelete{user, daysSinceLastUse}

			usersToDelete = append(usersToDelete, deleteUser)
			//fmt.Printf("\nAdding %s to remove list: %d days since last active, remove after %d days inactive\n", user.Username, daysSinceLastUse, *daysToDelete)
		}

		// confirm and remove users
		if confirmed, _ := confirmUserRemoval(usersToDelete); !confirmed {
			fmt.Println("Aborting removal")
			return nil
		} else {
			fmt.Println("REMOVING USERS")
			for _, user := range usersToDelete {
				if err := removeUser(user.User, client, ctx); err != nil {
					return err
				}
				if *toEmail {
					sendEmail(user.User)
				}
			}
		}

		return nil
	}

	// Register the command.
	usersCommands = append(usersCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

func computeDaysSinceLastUse(user User) (timeDiff int, wasLastActive bool, _ error) {
	timeNow := time.Now()
	// handle for null lastActiveTime returned from
	if user.UsageStatistics.LastActiveTime == "" {
		wasLastActive = false
		return 0, wasLastActive, nil
	}
	timeLast, err := time.Parse(time.RFC3339, user.UsageStatistics.LastActiveTime)
	if err != nil {
		fmt.Printf("failed to parse lastActive time: %s", err)
		return 0, false, err
	}
	timeDiff = int(timeNow.Sub(timeLast).Hours() / 24)

	return timeDiff, true, err
}

func removeUser(user User, client api.Client, ctx context.Context) error {
	query := `mutation DeleteUser(
  $user: ID!
) {
  deleteUser(
    user: $user
  ) {
    alwaysNil
  }
}`
	vars := map[string]interface{}{
		"user": user.ID,
	}
	if ok, err := client.NewRequest(query, vars).Do(ctx, nil); err != nil || !ok {
		return err
	}
	return nil
}

type UserToDelete struct {
	User             User
	DaysSinceLastUse int
}

func confirmUserRemoval(usersToRemove []UserToDelete) (bool, error) {
	fmt.Printf("Users to remove from instance at %s \n\t\t(Username|DisplayName|Email|DaysSinceLastActive)\n", cfg.Endpoint)
	for _, user := range usersToRemove {
		if len(user.User.Emails) > 0 {
			fmt.Printf("\t\t%s  %s  %s  %d\n", user.User.Username, user.User.DisplayName, user.User.Emails[0].Email, user.DaysSinceLastUse)
		} else {
			fmt.Printf("\t\t%s  %s  %d\n", user.User.Username, user.User.DisplayName, user.DaysSinceLastUse)
		}
	}
	input := ""
	for strings.ToLower(input) != "y" && strings.ToLower(input) != "n" {
		fmt.Printf("Do you  wish to proceed with user removal [y/N]: ")
		if _, err := fmt.Scanln(&input); err != nil {
			return false, err
		}
	}
	return strings.ToLower(input) == "y", nil
}

func sendEmail(user User) error {
	fmt.Printf("This sent an email to %s", user.Emails[0].Email)
	return nil
}
