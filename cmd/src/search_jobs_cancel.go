package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

const CancelSearchJobMutation = `mutation CancelSearchJob($id: ID!) {
	cancelSearchJob(id: $id) {
		alwaysNil
	}
}`

// init registers the 'cancel' subcommand for search jobs, which allows users to cancel
// a running search job by its ID. It sets up the command's flag parsing, usage information,
// and handles the GraphQL mutation to cancel the specified search job.
func init() {
	usage := `
Examples:

  Cancel a search job:

    $ src search-jobs cancel --id U2VhcmNoSm9iOjY5
`
	flagSet := flag.NewFlagSet("cancel", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src search-jobs %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		idFlag = flagSet.String("id", "", "ID of the search job to cancel")
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

		if *idFlag == "" {
			return cmderrors.Usage("must provide a search job ID")
		}

		query := CancelSearchJobMutation

		var result struct {
			CancelSearchJob struct {
				AlwaysNil bool
			}
		}

		if ok, err := client.NewRequest(query, map[string]interface{}{
			"id": *idFlag,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}
		fmt.Fprintf(flagSet.Output(), "Search job %s canceled successfully\n", *idFlag)
		return nil
	}

	searchJobsCommands = append(searchJobsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
