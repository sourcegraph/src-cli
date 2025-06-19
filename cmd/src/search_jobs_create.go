package main

import (
	"context"
	"flag"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

// GraphQL query and mutation constants
const (
	// createSearchJobQuery defines the GraphQL mutation for creating search jobs
	createSearchJobQuery = `mutation CreateSearchJob($query: String!) {
		createSearchJob(query: $query) {
			...SearchJobFields
		}
	}` + searchJobFragment

	// validateSearchJobQuery defines the GraphQL query for validating search queries
	validateSearchJobQuery = `query ValidateSearchJob($query: String!) {
		validateSearchJob(query: $query) { alwaysNil }
	}`
)

// validateSearchQuery validates a search query with the server
func validateSearchQuery(client api.Client, query string) error {
	var validateResult struct{}

	if ok, err := client.NewRequest(validateSearchJobQuery, map[string]any{
		"query": query,
	}).Do(context.Background(), &validateResult); err != nil || !ok {
		return err
	}

	return nil
}

// createSearchJob creates a new search job with the given query
func createSearchJob(client api.Client, query string) (*SearchJob, error) {
	var result struct {
		CreateSearchJob *SearchJob `json:"createSearchJob"`
	}

	if err := validateSearchQuery(client, query); err != nil {
		return nil, err
	}

	if ok, err := client.NewRequest(createSearchJobQuery, map[string]any{
		"query": query,
	}).Do(context.Background(), &result); !ok {
		return nil, err
	}

	return result.CreateSearchJob, nil
}

// init registers the "search-jobs create" subcommand.
func init() {
	usage := `
	Examples:
	
	  Create a search job:
	
		$ src search-jobs create "repo:^github\.com/sourcegraph/sourcegraph$ sort:indexed-desc"
		
	  Create a search job and display specific columns:
		
		$ src search-jobs create "repo:sourcegraph" -c id,state,username
		
	  Create a search job and output in JSON format:
		
		$ src search-jobs create "repo:sourcegraph" -json
		
	  Available columns are: id, query, state, username, createdat, startedat, finishedat, 
	  url, logurl, total, completed, failed, inprogress
	`

	// Use the builder pattern for command creation
	cmd := newSearchJobCommand("create", usage)

	cmd.build(func(flagSet *flag.FlagSet, apiFlags *api.Flags, columns []string, asJSON bool, client api.Client) error {

		if flagSet.NArg() != 1 {
			return cmderrors.Usage("must provide a query")
		}

		query := flagSet.Arg(0)

		job, err := createSearchJob(client, query)
		if err != nil {
			return err
		}

		if apiFlags.GetCurl() {
			return nil
		}

		return displaySearchJob(job, columns, asJSON)
	})
}
