package main

import (
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

// restartSearchJob restarts a search job with the same query as the original
func restartSearchJob(client api.Client, jobID string) (*SearchJob, error) {
	originalJob, err := getSearchJob(client, jobID)
	if err != nil {
		return nil, err
	}

	if originalJob == nil {
		return nil, fmt.Errorf("no job found with ID %s", jobID)
	}

	query := originalJob.Query

	return createSearchJob(client, query)
}

// init registers the "restart" subcommand for search jobs, which allows restarting
// a search job by its ID. It sets up command-line flags for job ID and output formatting,
// validates the search job query, and creates a new search job with the same query
// as the original job.
func init() {
	usage := `
	Examples:
	
	  Restart a search job by ID:
	
		$ src search-jobs restart U2VhcmNoSm9iOjY5
		
	  Restart a search job and display specific columns:
		
		$ src search-jobs restart U2VhcmNoSm9iOjY5 -c id,state,query
		
	  Restart a search job and output in JSON format:
		
		$ src search-jobs restart U2VhcmNoSm9iOjY5 -json
		
	  Available columns are: id, query, state, username, createdat, startedat, finishedat, 
	  url, logurl, total, completed, failed, inprogress
	`

	// Use the builder pattern for command creation
	cmd := NewSearchJobCommand("restart", usage)

	cmd.Build(func(flagSet *flag.FlagSet, apiFlags *api.Flags, columns []string, asJSON bool) error {
		// Validate job ID
		if flagSet.NArg() != 1 {
			return cmderrors.Usage("must provide a job ID")
		}
		jobID := flagSet.Arg(0)

		// Get the client
		client := createSearchJobsClient(flagSet, apiFlags)

		// Restart the job
		newJob, err := restartSearchJob(client, jobID)
		if err != nil {
			return err
		}

		// Display the new job
		return displaySearchJob(newJob, columns, asJSON)
	})
}
