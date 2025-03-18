package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
)

// GraphQL mutation constants
const deleteSearchJobQuery = `mutation DeleteSearchJob($id: ID!) {
	deleteSearchJob(id: $id) {
		alwaysNil
	}
}`

// deleteSearchJob deletes a search job with the given ID
func deleteSearchJob(client api.Client, jobID string) error {
	var result struct {
		DeleteSearchJob struct {
			AlwaysNil bool
		}
	}

	if ok, err := client.NewRequest(deleteSearchJobQuery, map[string]interface{}{
		"id": jobID,
	}).Do(context.Background(), &result); err != nil || !ok {
		return err
	}

	return nil
}

// displaySuccessMessage outputs a success message for the deleted job
func displaySuccessMessage(out *flag.FlagSet, jobID string) {
	fmt.Fprintf(out.Output(), "Search job %s deleted successfully\n", jobID)
}

// init registers the 'delete' subcommand for search-jobs which allows users to delete
// a search job by its ID. The command requires a search job ID to be provided via
// the -id flag and will make a GraphQL mutation to delete the specified job.
func init() {
	usage := `deletes a search job.
	Examples:
	
	  Delete a search job by ID:
	
		$ src search-jobs delete U2VhcmNoSm9iOjY5
	
	Arguments:
	  The ID of the search job to delete.
	
	The delete command permanently removes a search job and outputs a confirmation message.
	`

	// Use the builder pattern for command creation
	cmd := NewSearchJobCommand("delete", usage)

	cmd.Build(func(flagSet *flag.FlagSet, apiFlags *api.Flags, columns []string, asJSON bool) error {
		// Validate job ID using the shared function from search_jobs_get.go
		jobID, err := validateJobID(flagSet.Args())
		if err != nil {
			return err
		}

		// Get the client
		client := createSearchJobsClient(flagSet, apiFlags)

		// Send deletion request
		if err := deleteSearchJob(client, jobID); err != nil {
			return err
		}

		// Output success message
		displaySuccessMessage(flagSet, jobID)
		return nil
	})
}
