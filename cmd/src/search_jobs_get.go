package main

import (
	"context"
	"flag"

	"github.com/sourcegraph/src-cli/internal/api"
)

// GraphQL query constants
const getSearchJobQuery = `query SearchJob($id: ID!) {
    node(id: $id) {
        ... on SearchJob {
            ...SearchJobFields
        }
    }
}
`

// getSearchJob fetches a search job by ID
func getSearchJob(client api.Client, id string) (*SearchJob, error) {
	query := getSearchJobQuery + searchJobFragment

	var result struct {
		Node *SearchJob
	}

	if ok, err := client.NewRequest(query, map[string]interface{}{
		"id": api.NullString(id),
	}).Do(context.Background(), &result); err != nil || !ok {
		return nil, err
	}

	return result.Node, nil
}

// init registers the "get" subcommand for search-jobs
func init() {
	usage := `
	Examples:
	
	  Get a search job by ID:
	
		$ src search-jobs get U2VhcmNoSm9iOjY5
		
	  Get a search job with specific columns:
		
		$ src search-jobs get U2VhcmNoSm9iOjY5 -c id,state,username
		
	  Get a search job in JSON format:
		
		$ src search-jobs get U2VhcmNoSm9iOjY5 -json
		
	  Available columns are: id, query, state, username, createdat, startedat, finishedat, 
	  url, logurl, total, completed, failed, inprogress
	`

	// Use the builder pattern for command creation
	cmd := NewSearchJobCommand("get", usage)

	cmd.Build(func(flagSet *flag.FlagSet, apiFlags *api.Flags, columns []string, asJSON bool) error {
		// Get the client using the centralized function
		client := createSearchJobsClient(flagSet, apiFlags)

		// Validate that a job ID was provided
		id, err := validateJobID(flagSet.Args())
		if err != nil {
			return err
		}

		// Get the search job
		job, err := getSearchJob(client, id)
		if err != nil {
			return err
		}

		// Display the job with selected columns or as JSON
		return displaySearchJob(job, columns, asJSON)
	})
}
