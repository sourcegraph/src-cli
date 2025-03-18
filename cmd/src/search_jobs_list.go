package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

// GraphQL query constants
const listSearchJobsQuery = `query SearchJobs($first: Int!, $descending: Boolean!, $orderBy: SearchJobsOrderBy!) {
    searchJobs(first: $first, orderBy: $orderBy, descending: $descending) {
        nodes {
            ...SearchJobFields
        }
    }
}
`

// validOrderByValues defines the allowed values for the order-by flag
var validOrderByValues = map[string]bool{
	"QUERY":      true,
	"CREATED_AT": true,
	"STATE":      true,
}

// listSearchJobs fetches search jobs based on the provided parameters
func listSearchJobs(client api.Client, limit int, descending bool, orderBy string) ([]SearchJob, error) {
	query := listSearchJobsQuery + searchJobFragment

	var result struct {
		SearchJobs struct {
			Nodes []SearchJob
		}
	}

	if ok, err := client.NewRequest(query, map[string]interface{}{
		"first":      limit,
		"descending": descending,
		"orderBy":    orderBy,
	}).Do(context.Background(), &result); err != nil || !ok {
		return nil, err
	}

	return result.SearchJobs.Nodes, nil
}

// validateListFlags checks if the provided flags are valid
func validateListFlags(limit int, orderBy string) error {
	if limit < 1 {
		return cmderrors.Usage("limit flag must be greater than 0")
	}

	if !validOrderByValues[orderBy] {
		return cmderrors.Usage("order-by must be one of: QUERY, CREATED_AT, STATE")
	}

	return nil
}

// init registers the "list" subcommand for search-jobs which displays search jobs
// based on the provided filtering and formatting options.
func init() {
	usage := `
	Examples:
	
	  List all search jobs:
	
		$ src search-jobs list
	
	  List all search jobs in ascending order:
	
		$ src search-jobs list --asc
	
	  Limit the number of search jobs returned:
	
		$ src search-jobs list --limit 5
	
	  Order search jobs by a field (must be one of: QUERY, CREATED_AT, STATE):
	
		$ src search-jobs list --order-by QUERY
		
	  Select specific columns to display:
	  
		$ src search-jobs list -c id,state,username,createdat
		
	  Output results as JSON:
	  
		$ src search-jobs list -json
		
	  Combine options:
	  
		$ src search-jobs list --limit 10 --order-by STATE --asc -c id,query,state
		
	  Available columns are: id, query, state, username, createdat, startedat, finishedat, 
	  url, logurl, total, completed, failed, inprogress
	`

	// Use the builder pattern for command creation
	cmd := newSearchJobCommand("list", usage)

	// Add list-specific flags
	limitFlag := cmd.Flags.Int("limit", 10, "Limit the number of search jobs returned")
	ascFlag := cmd.Flags.Bool("asc", false, "Sort search jobs in ascending order")
	orderByFlag := cmd.Flags.String("order-by", "CREATED_AT", "Sort search jobs by a field")

	cmd.build(func(flagSet *flag.FlagSet, apiFlags *api.Flags, columns []string, asJSON bool) error {
		// Get the client using the centralized function
		client := createSearchJobsClient(flagSet, apiFlags)

		// Validate flags
		if err := validateListFlags(*limitFlag, *orderByFlag); err != nil {
			return err
		}

		// Fetch search jobs
		jobs, err := listSearchJobs(client, *limitFlag, !*ascFlag, *orderByFlag)
		if err != nil {
			return err
		}

		// Handle no results case
		if len(jobs) == 0 {
			return cmderrors.ExitCode(1, fmt.Errorf("no search jobs found"))
		}

		// Display the results with the selected columns or as JSON
		return displaySearchJobs(jobs, columns, asJSON)
	})
}
