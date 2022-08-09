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
		days     = flagSet.Int("d", 1000, "Returns the first n users from the list. (use -1 for unlimited)")
		apiFlags = api.NewFlags(flagSet)
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
			delete_users_not_active_in(user.UsageStatistics.LastActiveTime, *days)
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

func delete_users_not_active_in(usage string, days_threshold int) error {
	timeNow := time.Now()
	timeLast, err := time.Parse(time.RFC3339, usage)
	if err != nil {
		fmt.Printf("failed to parse lastActive time: %s", err)
	}
	timeDiff := timeNow.Sub(timeLast)
	if err != nil {
		fmt.Printf("failed to diff lastActive to current time: %s", err)
	}

	fmt.Printf("Time now: %s\nLast active: %s\nTime diff: %d\n\n", timeNow, timeLast, int(timeDiff.Hours()/24))
	query := `mutation DeleteUser(
  $user: ID!
) {
  deleteUser(
    user: $user
  ) {
    alwaysNil
  }
}`
	fmt.Printf("%s -- %d\n", usage, days_threshold)
	if days_threshold == 0 {
		fmt.Println(query)
	}
	return nil
}
