package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/sourcegraph/src-cli/internal/api"
)

func init() {
	usage := `
Examples:

  List teams:

    	$ src teams list

  List teams whose names match the query:

    	$ src teams list -query='myquery'

  List *all* teams (may be slow!):

    	$ src teams list -first='-1'

`

	flagSet := flag.NewFlagSet("list", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src teams %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}
	var (
		firstFlag  = flagSet.Int("first", 1000, "Returns the first n teams from the list. (use -1 for unlimited)")
		queryFlag  = flagSet.String("query", "", `Returns teams whose name or displayname match the query. (e.g. "engineering")`)
		formatFlag = flagSet.String("f", "{{.Name}}", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.Name}}: {{.DisplayName}}" or "{{.|json}}")`)
		jsonFlag   = flagSet.Bool("json", false, `Format for the output as json`)
		apiFlags   = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		tmpl, err := parseTemplate(*formatFlag)
		if err != nil {
			return err
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())

		query := `query Teams(
	$first: Int,
	$query: String
) {
	teams(
		first: $first,
		query: $query
	) {
		nodes {
			...TeamFields
		}
		
	}
}` + teamFragment

		var result struct {
			Teams struct {
				Nodes []Team
			}
		}
		if ok, err := client.NewRequest(query, map[string]interface{}{
			"first": api.NullInt(*firstFlag),
			"query": api.NullString(*queryFlag),
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		if jsonFlag != nil && *jsonFlag {
			json.NewEncoder(os.Stdout).Encode(result.Teams.Nodes)
			return nil
		}

		for _, t := range result.Teams.Nodes {
			if err := execTemplate(tmpl, t); err != nil {
				return err
			}
		}
		return nil
	}

	// Register the command.
	teamsCommands = append(teamsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
