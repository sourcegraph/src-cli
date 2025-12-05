package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
)

// GraphQL mutation constants
const cancelSearchJobMutation = `mutation CancelSearchJob($id: ID!) {
	cancelSearchJob(id: $id) {
		alwaysNil
	}
}`

// cancelSearchJob cancels a search job with the given ID
func cancelSearchJob(client api.Client, jobID string) error {
	var result struct {
		CancelSearchJob struct {
			AlwaysNil bool
		}
	}

	if ok, err := client.NewRequest(cancelSearchJobMutation, map[string]any{
		"id": jobID,
	}).Do(context.Background(), &result); err != nil || !ok {
		return err
	}

	return nil
}

// displayCancelSuccessMessage outputs a success message for the canceled job
func displayCancelSuccessMessage(out *flag.FlagSet, jobID string) {
	fmt.Fprintf(out.Output(), "Search job %s canceled successfully\n", jobID)
}

// init registers the 'cancel' subcommand for search jobs, which allows users to cancel
// a running search job by its ID. It sets up the command's flag parsing, usage information,
// and handles the GraphQL mutation to cancel the specified search job.
func init() {
	usage := `
	Examples:
	
	  Cancel a search job by ID:
	
		$ src search-jobs cancel U2VhcmNoSm9iOjY5
	
	Arguments:
	  The ID of the search job to cancel.
	
	The cancel command stops a running search job and outputs a confirmation message.
	`

	cmd := newSearchJobCommand("cancel", usage)

	cmd.build(func(flagSet *flag.FlagSet, apiFlags *api.Flags, columns []string, asJSON bool, client api.Client) error {

		jobID, err := validateJobID(flagSet.Args())
		if err != nil {
			return err
		}

		if err := cancelSearchJob(client, jobID); err != nil {
			return err
		}

		if apiFlags.GetCurl() {
			return nil
		}

		displayCancelSuccessMessage(flagSet, jobID)

		return nil
	})
}
