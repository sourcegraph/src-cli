package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

// fetchJobResults retrieves results for a search job from its results URL
func fetchJobResults(jobID string, resultsURL string) (io.ReadCloser, error) {
	if resultsURL == "" {
		return nil, fmt.Errorf("no results URL found for search job %s", jobID)
	}

	// Prepare HTTP request for results
	req, err := http.NewRequest("GET", resultsURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "token "+cfg.AccessToken)

	// Execute request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// outputResults writes results to either a file or stdout
func outputResults(results io.Reader, outputPath string) error {
	if outputPath != "" {
		// Write to file
		file, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()

		_, err = io.Copy(file, results)
		if err != nil {
			return fmt.Errorf("failed to write to output file: %w", err)
		}
		return nil
	}

	// Write to stdout
	_, err := io.Copy(os.Stdout, results)
	return err
}

// init registers the "results" subcommand for search jobs, which allows users to view
// results for a specific search job by its ID. The command requires a search job ID
// and uses the configured API client to fetch and display the results.
func init() {
	usage := `retrieves the results of a search job in JSON Lines (jsonl) format.
	Examples:
	
		Get the results of a search job:
		  $ src search-jobs results U2VhcmNoSm9iOjY5
	
		Save search results to a file:
		  $ src search-jobs results U2VhcmNoSm9iOjY5 -out results.jsonl
	
	The results command retrieves the raw search results in JSON Lines format. 
	Each line contains a single JSON object representing a search result. The data 
	will be displayed on stdout or written to the file specified with -out.
	`

	// Use the builder pattern for command creation
	cmd := NewSearchJobCommand("results", usage)

	// Add results-specific flag
	outFlag := cmd.Flags.String("out", "", "File path to save the results (optional)")

	cmd.Build(func(flagSet *flag.FlagSet, apiFlags *api.Flags, columns []string, asJSON bool) error {
		// Validate job ID
		if flagSet.NArg() != 1 {
			return cmderrors.Usage("must provide a search job ID")
		}
		jobID := flagSet.Arg(0)

		// Get the client and fetch job details
		client := createSearchJobsClient(flagSet, apiFlags)
		job, err := getSearchJob(client, jobID)
		if err != nil {
			return err
		}

		if job == nil {
			return fmt.Errorf("no job found with ID %s", jobID)
		}

		// Fetch results
		resultsData, err := fetchJobResults(jobID, job.URL)
		if err != nil {
			return err
		}
		defer resultsData.Close()

		// Output results
		return outputResults(resultsData, *outFlag)
	})
}
