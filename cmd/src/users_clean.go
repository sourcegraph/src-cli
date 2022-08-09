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
		days      = flagSet.Int("d", 365, "Returns the first n users from the list. (use -1 for unlimited)")
		noAdmin   = flagSet.Bool("no-admin", false, "Omit admin accounts from cleanup")
		sendEmail = flagSet.Bool("email", false, "send removed users an email")
		apiFlags  = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		flagSet.Parse(args)

		ctx := context.Background()
		client := cfg.apiClient(apiFlags, flagSet.Output())

		tmpl, err := parseTemplate("{{.Username}}  {{.SiteAdmin}} {{(index .Emails 0).Email}}")
		if err != nil {
			return err
		}
		vars := map[string]interface{}{
			"-d": api.NullInt(*days),
		}

		query := `
query Users($first: Int, $query: String) {
	users(first: $first, query: $query) {
		nodes {
			id
			username
			displayName
			siteAdmin
			organizations {
				nodes {
					id
					name
					displayName
				}
			}
			emails {
				email
				verified
			}
			usageStatistics {
				lastActiveTime
				lastActiveCodeHostIntegrationTime
			}
			url
		}
	}
}
`

		var result struct {
			Users struct {
				Nodes []User
			}
		}
		if ok, err := client.NewRequest(query, vars).Do(ctx, &result); err != nil || !ok {
			return err
		}

		for _, user := range result.Users.Nodes {
			fmt.Printf("PRINT: %v, %v", *noAdmin, *sendEmail)
			daysSinceLastUse, err := timeSinceLastUse(user, *days)
			if err != nil {
				fmt.Print(err)
			}
			if daysSinceLastUse >= *days {
				removeUser(user)
			}
			if err := execTemplate(tmpl, user); err != nil {
				return err
			}
		}
		return err
	}

	// Register the command.
	usersCommands = append(usersCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

func timeSinceLastUse(user User, daysToDelete int) (int, error) {
	timeNow := time.Now()
	if user.UsageStatistics.LastActiveTime == "" {
		fmt.Printf("%s at (%s) has no lastActive value\n", user.Username, user.Emails[0].Email)
		return 0, nil
	}
	timeLast, err := time.Parse(time.RFC3339, user.UsageStatistics.LastActiveTime)
	if err != nil {
		fmt.Printf("failed to parse lastActive time: %s", err)
	}
	timeDiff := int(timeNow.Sub(timeLast).Hours() / 24)
	if err != nil {
		fmt.Printf("failed to diff lastActive to current time: %s", err)
	}

	fmt.Printf("Time now: %s\nLast active: %s\nTime diff: %d\n\n", timeNow, timeLast, timeDiff)
	return timeDiff, err
}

func removeUser(user User) error {
	query := `mutation DeleteUser(
  $user: ID!
) {
  deleteUser(
    user: $user
  ) {
    alwaysNil
  }
}`
	fmt.Printf("Deleted user: %s\n%s", user.Username, query)
	return nil
}

func sendEmail(user *User) error {
	fmt.Printf("This sent an email to %s", user.Emails[0].Email)
	return nil
}
