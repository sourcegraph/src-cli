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

	cmd := newSearchJobCommand("get", usage)

	cmd.build(func(flagSet *flag.FlagSet, apiFlags *api.Flags, columns []string, asJSON bool, client api.Client) error {

		id, err := validateJobID(flagSet.Args())
		if err != nil {
			return err
		}

		job, err := getSearchJob(client, id)
		if err != nil {
			return err
		}

		if apiFlags.GetCurl() {
			return nil
		}

		// Display the job with selected columns or as JSON
		return displaySearchJob(job, columns, asJSON)
	})
}
