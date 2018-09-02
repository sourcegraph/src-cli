package main

import (
	"flag"
	"fmt"
)

func init() {
	usage := `
Examples:

  List users:

    	$ src users list

  List *all* users (may be slow!):

    	$ src users list -first='-1'

  List users whose names match the query:

    	$ src users list -query='myquery'

  List all users with the "foo" tag:

    	$ src users list -tag=foo

`

	flagSet := flag.NewFlagSet("list", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src users %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		firstFlag  = flagSet.Int("first", 1000, "Returns the first n users from the list. (use -1 for unlimited)")
		queryFlag  = flagSet.String("query", "", `Returns users whose names match the query. (e.g. "alice")`)
		tagFlag    = flagSet.String("tag", "", `Returns users with the given tag.`)
		formatFlag = flagSet.String("f", "{{.Username}}", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Username}} ({{.DisplayName}})" or "{{.|json}}")`)
		apiFlags   = newAPIFlags(flagSet)
	)

	handler := func(args []string) error {
		flagSet.Parse(args)

		tmpl, err := parseTemplate(*formatFlag)
		if err != nil {
			return err
		}

		query := `query Users(
  $first: Int,
  $query: String,
  $tag: String,
) {
  users(
    first: $first,
    query: $query,
    tag: $tag,
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
		return (&apiRequest{
			query: query,
			vars: map[string]interface{}{
				"first": nullInt(*firstFlag),
				"query": nullString(*queryFlag),
				"tag":   nullString(*tagFlag),
			},
			result: &result,
			done: func() error {
				for _, user := range result.Users.Nodes {
					if err := execTemplate(tmpl, user); err != nil {
						return err
					}
				}
				return nil
			},
			flags: apiFlags,
		}).do()
	}

	// Register the command.
	usersCommands = append(usersCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
