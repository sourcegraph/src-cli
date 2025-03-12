package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/sourcegraph/src-cli/internal/api"
)

// init registers the 'logs' subcommand for search jobs, which allows users to view
// logs for a specific search job by its ID. The command requires a search job ID
// and uses the configured API client to fetch and display the logs.
func init() {
	usage := `retrieves the logs of a search job in CSV format.
Examples:

	View the logs of a search job:
	  $ src search-jobs logs -id 999

	Save the logs to a file:
	  $ src search-jobs logs U2VhcmNoSm9iOjY5 -out logs.csv

`
	flagSet := flag.NewFlagSet("logs", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src search-jobs %s':\n", flagSet.Name())
		flagSet.PrintDefaults()
		fmt.Println(usage)
	}

	var (
		idFlag   = flagSet.String("id", "", "ID of the search job to view logs for")
		outFlag  = flagSet.String("out", "", "File path to save the logs (optional)")
		apiFlags = api.NewFlags(flagSet)
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		client := api.NewClient(api.ClientOpts{
			Endpoint:    cfg.Endpoint,
			AccessToken: cfg.AccessToken,
			Out:         flagSet.Output(),
			Flags:       apiFlags,
		})

		job, err := getSearchJob(client, *idFlag)
		if err != nil {
			return err
		}

		if job == nil || job.LogURL == "" {
			return fmt.Errorf("no logs URL found for search job %s", *idFlag)
		}

		req, err := http.NewRequest("GET", job.LogURL, nil)
		if err != nil {
			return err
		}

		req.Header.Add("Authorization", "token "+cfg.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if *outFlag != "" {

			file, err := os.Create(*outFlag)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}

			defer file.Close()

			_, err = io.Copy(file, resp.Body)
			if err != nil {
				return fmt.Errorf("failed to write to output file: %w", err)
			}
			return nil
		}

		_, err = io.Copy(os.Stdout, resp.Body)
		return err
	}

	searchJobsCommands = append(searchJobsCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}
