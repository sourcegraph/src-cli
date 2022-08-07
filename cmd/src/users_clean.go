package main

import (
	"context"
	"flag"
	"fmt"

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

		tmpl, err := parseTemplate("{{.Username}}  {{.SiteAdmin}}")
		if err != nil {
			return err
		}
		vars := map[string]interface{}{
			"-d": api.NullInt(*days),
		}

		query := `query Users(
  $first: Int,
  $query: String,
) {
  users(
first: $first,
    query: $query,
  ) {
    nodes {
      ...UserFields
    }
  }
}` + userFragment

		var result struct {
			Users struct {
				Nodes []User
			}
		}
		if ok, err := client.NewRequest(query, vars).Do(ctx, &result); err != nil || !ok {
			return err
		}

		for _, user := range result.Users.Nodes {
			if err := execTemplate(tmpl, user); err != nil {
				return err
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
