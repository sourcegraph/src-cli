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

	req, err := http.NewRequest("GET", logURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "token "+cfg.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func outputLogs(logs io.Reader, outputPath string) error {
	if outputPath != "" {
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

	_, err := io.Copy(os.Stdout, logs)
	return err
}

// init registers the 'logs' subcommand for search jobs, which allows users to view
// logs for a specific search job by its ID. The command requires a search job ID
// and uses the configured API client to fetch and display the logs.
func init() {
	usage := `
	Examples:
	
		View the logs of a search job:
		  $ src search-jobs logs U2VhcmNoSm9iOjY5
	
		Save the logs to a file:
		  $ src search-jobs logs U2VhcmNoSm9iOjY5 -out logs.csv
	
	The logs command retrieves the raw log data in CSV format. The data will be 
	displayed on stdout or written to the file specified with -out.
	`

	cmd := newSearchJobCommand("logs", usage)

	outFlag := cmd.Flags.String("out", "", "File path to save the logs (optional)")

	cmd.build(func(flagSet *flag.FlagSet, apiFlags *api.Flags, columns []string, asJSON bool, client api.Client) error {
		if flagSet.NArg() == 0 {
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

		logsData, err := fetchJobLogs(jobID, job.LogURL)
		if err != nil {
			return err
		}

		if apiFlags.GetCurl() {
			return nil
		}

		defer logsData.Close()

		return outputLogs(logsData, *outFlag)
	})
}
