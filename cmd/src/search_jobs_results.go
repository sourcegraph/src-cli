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
func fetchJobResults(client api.Client, jobID string, resultsURL string) (io.ReadCloser, error) {
	if resultsURL == "" {
		return nil, fmt.Errorf("no results URL found for search job %s", jobID)
	}

	req, err := http.NewRequest("GET", resultsURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "token "+cfg.AccessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// outputResults writes results to either a file or stdout
func outputResults(results io.Reader, outputPath string) error {
	if outputPath != "" {

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

	_, err := io.Copy(os.Stdout, results)
	return err
}

// init registers the "results" subcommand for search jobs, which allows users to view
// results for a specific search job by its ID. The command requires a search job ID
// and uses the configured API client to fetch and display the results.
func init() {
	usage := `
	Examples:
	
		Get the results of a search job:
		  $ src search-jobs results U2VhcmNoSm9iOjY5
	
		Save search results to a file:
		  $ src search-jobs results U2VhcmNoSm9iOjY5 -out results.jsonl
	
	The results command retrieves the raw search results in JSON Lines format. 
	Each line contains a single JSON object representing a search result. The data 
	will be displayed on stdout or written to the file specified with -out.
	`

	cmd := newSearchJobCommand("results", usage)

	outFlag := cmd.Flags.String("out", "", "File path to save the results (optional)")

	cmd.build(func(flagSet *flag.FlagSet, apiFlags *api.Flags, columns []string, asJSON bool, client api.Client) error {
		if flagSet.NArg() != 1 {
			return cmderrors.Usage("must provide a search job ID")
		}
		jobID := flagSet.Arg(0)

		job, err := getSearchJob(client, jobID)
		if err != nil {
			return err
		}

		if job == nil {
			return fmt.Errorf("no job found with ID %s", jobID)
		}

		resultsData, err := fetchJobResults(client, jobID, job.URL)
		if err != nil {
			return err
		}

		if apiFlags.GetCurl() {
			return nil
		}

		defer resultsData.Close()

		return outputResults(resultsData, *outFlag)
	})
}
