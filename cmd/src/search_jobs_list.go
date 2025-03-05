package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

const ListSearchJobsQuery = `query SearchJobs($first: Int!, $descending: Boolean!, $orderBy: SearchJobsOrderBy!) {
    searchJobs(first: $first, orderBy: $orderBy, descending: $descending) {
        nodes {
            ...SearchJobFields
        }
    }
}
`

// init registers the "list" subcommand for search-jobs which displays search jobs
// based on the provided filtering and formatting options. It supports pagination,
// sorting by different fields, and custom output formatting using Go templates.
func init() {
	usage := `
Examples:

  List all search jobs:

    $ src search-jobs list

  List all search jobs in ascending order:

    $ src search-jobs list -asc

  Limit the number of search jobs returned:

    $ src search-jobs list -limit 5

  Order search jobs by a field (must be one of: QUERY, CREATED_AT, STATE):

    $ src search-jobs list -order-by QUERY
`
	flagSet := flag.NewFlagSet("list", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src search-jobs %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		formatFlag  = flagSet.String("f", "{{searchJobIDNumber .ID}}: {{.Creator.Username}} {{.State}}", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Creator.Username}} ({{.Query}})" or "{{.|json}}")`)
		limitFlag   = flagSet.Int("limit", 10, "Limit the number of search jobs returned")
		ascFlag     = flagSet.Bool("asc", false, "Sort search jobs in ascending order")
		orderByFlag = flagSet.String("order-by", "CREATED_AT", "Sort search jobs by a field")
		apiFlags    = api.NewFlags(flagSet)
	)

	validOrderBy := map[string]bool{
		"QUERY":      true,
		"CREATED_AT": true,
		"STATE":      true,
	}

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := api.NewClient(api.ClientOpts{
			Endpoint:    cfg.Endpoint,
			AccessToken: cfg.AccessToken,
			Out:         flagSet.Output(),
			Flags:       apiFlags,
		})

		if *limitFlag < 1 {
			return cmderrors.Usage("limit flag must be greater than 0")
		}

		if !validOrderBy[*orderByFlag] {
			return cmderrors.Usage("order-by must be one of: QUERY, CREATED_AT, STATE")
		}

		tmpl, err := parseTemplate(*formatFlag)
		if err != nil {
			return err
		}

		query := ListSearchJobsQuery + SearchJobFragment

		var result struct {
			SearchJobs struct {
				Nodes []SearchJob
			}
		}

		if ok, err := client.NewRequest(query, map[string]interface{}{
			"first":      *limitFlag,
			"descending": !*ascFlag,
			"orderBy":    *orderByFlag,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}

		if len(result.SearchJobs.Nodes) == 0 {
			return cmderrors.ExitCode(1, fmt.Errorf("no search jobs found"))
		}

		for _, job := range result.SearchJobs.Nodes {
			if err := execTemplate(tmpl, job); err != nil {
				return err
			}
		}

		return nil
	}

	searchJobsCommands = append(searchJobsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
