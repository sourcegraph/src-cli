package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

const DeleteSearchJobQuery = `mutation DeleteSearchJob($id: ID!) {
	deleteSearchJob(id: $id) {
		alwaysNil
	}
}`

// init registers the 'delete' subcommand for search-jobs which allows users to delete
// a search job by its ID. The command requires a search job ID to be provided via
// the -id flag and will make a GraphQL mutation to delete the specified job.
func init() {
	usage := `
Examples:

  Delete a search job by ID:

    $ src search-jobs delete U2VhcmNoSm9iOjY5
`

	flagSet := flag.NewFlagSet("delete", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src search-jobs %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		idFlag = flagSet.String("id", "", "ID of the search job to delete")
		apiFlags = api.NewFlags(flagSet)
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

		var result struct {
			DeleteSearchJob struct {
				AlwaysNil bool
			}
		}

		if ok, err := client.NewRequest(DeleteSearchJobQuery, map[string]interface{}{
			"id": *idFlag,
		}).Do(context.Background(), &result); err != nil || !ok {
			return err
		}
		fmt.Fprintf(flagSet.Output(), "Search job %s deleted successfully\n", *idFlag)
		return nil
	}

	searchJobsCommands = append(searchJobsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
