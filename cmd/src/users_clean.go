package main

import (
	"context"
	"flag"
	"fmt"
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
		daysToDelete       = flagSet.Int("d", 365, "Returns the first n users from the list. (use -1 for unlimited)")
		noAdmin            = flagSet.Bool("no-admin", false, "Omit admin accounts from cleanup")
		toEmail            = flagSet.Bool("email", false, "send removed users an email")
		removeNoLastActive = flagSet.Bool("removeNeverActive", false, "removes users with null lastActive value")
		apiFlags           = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		flagSet.Parse(args)

		ctx := context.Background()
		client := cfg.apiClient(apiFlags, flagSet.Output())

		//tmpl, err := parseTemplate("{{.Username}}  {{.SiteAdmin}} {{(index .Emails 0).Email}}")
		//if err != nil {
		//	return err
		//}
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

		var result struct {
			Users struct {
				Nodes []User
			}
		}

		if ok, err := client.NewRequest(query, vars).Do(ctx, &result); err != nil || !ok {
			return err
		}

		usersToDelete := make([]User, 0)
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
			if daysSinceLastUse <= *daysToDelete {
				continue
			}

			usersToDelete = append(usersToDelete, user)
			fmt.Printf("\nAdding %s to remove list: %d days since last active, remove after %d days inactive\n", user.Username, daysSinceLastUse, *daysToDelete)
		}
		for _, user := range usersToDelete {
			if err := removeUser(user, client, ctx); err != nil {
				return err
			}
			if *toEmail {
				sendEmail(user)
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
	//TODO handle for null lastActiveTime = null
	if user.UsageStatistics.LastActiveTime == "" {
		fmt.Printf("\n%s has no lastActive value\n", user.Username)
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
	fmt.Printf("\nDeleted user %s: %s\n", user.ID, user.Username)
	return nil
}

func sendEmail(user User) error {
	fmt.Printf("This sent an email to %s", user.Emails[0].Email)
	return nil
}
