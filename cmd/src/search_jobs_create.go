package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

// ValidateSearchJobQuery defines the GraphQL query for validating search jobs
const ValidateSearchJobQuery = `query ValidateSearchJob($query: String!) {
    validateSearchJob(query: $query) {
        alwaysNil
    }
}`

// CreateSearchJobQuery defines the GraphQL mutation for creating search jobs
const CreateSearchJobQuery = `mutation CreateSearchJob($query: String!) {
    createSearchJob(query: $query) {
        ...SearchJobFields
    }
}` + SearchJobFragment

// init registers the "search-jobs create" subcommand. It allows users to create a search job
// with a specified query, validates the query before creation, and outputs the result in a
// customizable format. The command requires a search query and supports custom output formatting
// using Go templates.
func init() {
	usage := `
Examples:

  Create a search job:
  
    $ src search-jobs create -query "repo:^github\.com/sourcegraph/sourcegraph$ sort:indexed-desc"
`

	flagSet := flag.NewFlagSet("create", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src search-jobs %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		queryFlag = flagSet.String("query", "", "Search query")
		formatFlag = flagSet.String("f", "{{.ID}}: {{.Creator.Username}} {{.State}} ({{.Query}})", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Creator.Username}} ({{.Query}})" or "{{.|json}}")`)
		apiFlags  = api.NewFlags(flagSet)
	)

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

		tmpl, err := parseTemplate(*formatFlag)
		if err != nil {
			return err
		}

		if *queryFlag == "" {
			return cmderrors.Usage("must provide a query")
		}

		var validateResult struct {
			ValidateSearchJob interface{} `json:"validateSearchJob"`
		}

		if ok, err := client.NewRequest(ValidateSearchJobQuery, map[string]interface{}{
			"query": *queryFlag,
		}).Do(context.Background(), &validateResult); err != nil || !ok {
			return err
		}

		query := CreateSearchJobQuery

		var result struct {
			CreateSearchJob *SearchJob `json:"createSearchJob"`
		}

		if ok, err := client.NewRequest(query, map[string]interface{}{
			"query": *queryFlag,
		}).Do(context.Background(), &result); !ok {
			return err
		}

		return execTemplate(tmpl, result.CreateSearchJob)
	}

	searchJobsCommands = append(searchJobsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
