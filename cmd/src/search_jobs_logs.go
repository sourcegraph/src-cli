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

// fetchJobLogs retrieves logs for a search job from its log URL
func fetchJobLogs(jobID string, logURL string) (io.ReadCloser, error) {
	if logURL == "" {
		return nil, fmt.Errorf("no logs URL found for search job %s", jobID)
	}

	// Prepare HTTP request for logs
	req, err := http.NewRequest("GET", logURL, nil)
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

// outputLogs writes logs to either a file or stdout
func outputLogs(logs io.Reader, outputPath string) error {
	if outputPath != "" {
		// Write to file
		file, err := os.Create(outputPath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer file.Close()

		_, err = io.Copy(file, logs)
		if err != nil {
			return fmt.Errorf("failed to write to output file: %w", err)
		}
		return nil
	}

	// Write to stdout
	_, err := io.Copy(os.Stdout, logs)
	return err
}

// init registers the 'logs' subcommand for search jobs, which allows users to view
// logs for a specific search job by its ID. The command requires a search job ID
// and uses the configured API client to fetch and display the logs.
func init() {
	usage := `retrieves the logs of a search job in CSV format.
	Examples:
	
		View the logs of a search job:
		  $ src search-jobs logs U2VhcmNoSm9iOjY5
	
		Save the logs to a file:
		  $ src search-jobs logs U2VhcmNoSm9iOjY5 -out logs.csv
	
	The logs command retrieves the raw log data in CSV format. The data will be 
	displayed on stdout or written to the file specified with -out.
	`

	// Use the builder pattern for command creation
	cmd := NewSearchJobCommand("logs", usage)

	// Add logs-specific flag
	outFlag := cmd.Flags.String("out", "", "File path to save the logs (optional)")

	cmd.Build(func(flagSet *flag.FlagSet, apiFlags *api.Flags, columns []string, asJSON bool) error {
		// Validate job ID
		if flagSet.NArg() == 0 {
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

		// Fetch logs
		logsData, err := fetchJobLogs(jobID, job.LogURL)
		if err != nil {
			return err
		}
		defer logsData.Close()

		// Output logs
		return outputLogs(logsData, *outFlag)
	})
}
