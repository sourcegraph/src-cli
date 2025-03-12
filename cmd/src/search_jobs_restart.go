package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

// init registers the "restart" subcommand for search jobs, which allows restarting
// a search job by its ID. It sets up command-line flags for job ID and output formatting,
// validates the search job query, and creates a new search job with the same query
// as the original job.
func init() {
	usage := `
Examples:

  Restart a search job by ID:

    $ src search-jobs restart -id 999
`

	flagSet := flag.NewFlagSet("restart", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src search-jobs %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		idFlag     = flagSet.String("id", "", "ID of the search job to restart")
		formatFlag = flagSet.String("f", "{{searchJobIDNumber .ID}}: {{.Creator.Username}} {{.State}} ({{.Query}})", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Creator.Username}} ({{.Query}})" or "{{.|json}}")`)
		apiFlags   = api.NewFlags(flagSet)
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

		if *idFlag == "" {
			return cmderrors.Usage("must provide a job ID")
		}

		originalJob, err := getSearchJob(client, *idFlag)
		if err != nil {
			return err
		}
		query := originalJob.Query
		var validateResult struct {
			ValidateSearchJob struct {
				AlwaysNil bool `json:"alwaysNil"`
			} `json:"validateSearchJob"`
		}

		if ok, err := client.NewRequest(ValidateSearchJobQuery, map[string]interface{}{
			"query": query,
		}).Do(context.Background(), &validateResult); err != nil || !ok {
			return err
		}
		var result struct {
			CreateSearchJob *SearchJob `json:"createSearchJob"`
		}

		if ok, err := client.NewRequest(CreateSearchJobQuery, map[string]interface{}{
			"query": query,
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
