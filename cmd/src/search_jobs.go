package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

// searchJobFragment is a GraphQL fragment that defines the fields to be queried
// for a SearchJob. It includes the job's ID, query, state, creator information,
// timestamps, URLs, and repository statistics.
const searchJobFragment = `
fragment SearchJobFields on SearchJob {
    id
    query
    state
    creator {
        username
    }
    createdAt
    startedAt
    finishedAt
    URL
    logURL
    repoStats {
        total
        completed
        failed
        inProgress
    }
}`

// SearchJob represents a search job with its metadata, including the search query,
// execution state, creator information, timestamps, URLs, and repository statistics.
type SearchJob struct {
	ID      string
	Query   string
	State   string
	Creator struct {
		Username string
	}
	CreatedAt  string
	StartedAt  string
	FinishedAt string
	URL        string
	LogURL     string
	RepoStats  struct {
		Total      int
		Completed  int
		Failed     int
		InProgress int
	}
}

// availableColumns defines the available column names for output
var availableColumns = map[string]bool{
	"id":         true,
	"query":      true,
	"state":      true,
	"username":   true,
	"createdat":  true,
	"startedat":  true,
	"finishedat": true,
	"url":        true,
	"logurl":     true,
	"total":      true,
	"completed":  true,
	"failed":     true,
	"inprogress": true,
}

// defaultColumns defines the default columns to display
var defaultColumns = []string{"id", "username", "state", "query"}

// SearchJobCommandBuilder helps build search job commands with common flags and options
type SearchJobCommandBuilder struct {
	Name     string
	Usage    string
	Flags    *flag.FlagSet
	ApiFlags *api.Flags
}

// Global variables
var searchJobsCommands commander

// newSearchJobCommand creates a new search job command builder
func newSearchJobCommand(name string, usage string) *SearchJobCommandBuilder {
	flagSet := flag.NewFlagSet(name, flag.ExitOnError)
	return &SearchJobCommandBuilder{
		Name:     name,
		Usage:    usage,
		Flags:    flagSet,
		ApiFlags: api.NewFlags(flagSet),
	}
}

// build creates and registers the command
func (b *SearchJobCommandBuilder) build(handlerFunc func(*flag.FlagSet, *api.Flags, []string, bool, api.Client) error) {
	columnsFlag := b.Flags.String("c", strings.Join(defaultColumns, ","),
		"Comma-separated list of columns to display. Available: id,query,state,username,createdat,startedat,finishedat,url,logurl,total,completed,failed,inprogress")
	jsonFlag := b.Flags.Bool("json", false, "Output results as JSON for programmatic access")

	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src search-jobs %s':\n", b.Name)
		b.Flags.PrintDefaults()
		fmt.Println(b.Usage)
	}

	handler := func(args []string) error {
		if err := parseSearchJobsArgs(b.Flags, args); err != nil {
			return err
		}

		// Parse columns
		columns := parseColumns(*columnsFlag)

		client := createSearchJobsClient(b.Flags, b.ApiFlags)

		return handlerFunc(b.Flags, b.ApiFlags, columns, *jsonFlag, client)
	}

	searchJobsCommands = append(searchJobsCommands, &command{
		flagSet:   b.Flags,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

// parseColumns parses and validates the columns flag
func parseColumns(columnsFlag string) []string {
	if columnsFlag == "" {
		return defaultColumns
	}

	columns := strings.Split(columnsFlag, ",")
	var validColumns []string

	for _, col := range columns {
		col = strings.ToLower(strings.TrimSpace(col))
		if availableColumns[col] {
			validColumns = append(validColumns, col)
		}
	}

	if len(validColumns) == 0 {
		return defaultColumns
	}

	return validColumns
}

// createSearchJobsClient creates a reusable API client for search jobs commands
func createSearchJobsClient(out *flag.FlagSet, apiFlags *api.Flags) api.Client {
	return api.NewClient(api.ClientOpts{
		Endpoint:    cfg.Endpoint,
		AccessToken: cfg.AccessToken,
		Out:         out.Output(),
		Flags:       apiFlags,
	})
}

// parseSearchJobsArgs parses command arguments with the provided flag set
// and returns an error if parsing fails
func parseSearchJobsArgs(flagSet *flag.FlagSet, args []string) error {
	if err := flagSet.Parse(args); err != nil {
		return err
	}
	return nil
}

// validateJobID validates that a job ID was provided
func validateJobID(args []string) (string, error) {
	if len(args) != 1 {
		return "", cmderrors.Usage("must provide a search job ID")
	}
	return args[0], nil
}

// displaySearchJob formats and outputs a search job based on selected columns or JSON
func displaySearchJob(job *SearchJob, columns []string, asJSON bool) error {
	if asJSON {
		return outputAsJSON(job)
	}
	return outputAsColumns(job, columns)
}

// displaySearchJobs formats and outputs multiple search jobs
func displaySearchJobs(jobs []SearchJob, columns []string, asJSON bool) error {
	if asJSON {
		return outputAsJSON(jobs)
	}

	for _, job := range jobs {
		if err := outputAsColumns(&job, columns); err != nil {
			return err
		}
	}
	return nil
}

// outputAsJSON outputs data as JSON
func outputAsJSON(data any) error {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonBytes))
	return nil
}

// outputAsColumns outputs a search job as tab-delimited columns
func outputAsColumns(job *SearchJob, columns []string) error {
	values := make([]string, 0, len(columns))

	for _, col := range columns {
		switch col {
		case "id":
			values = append(values, job.ID)
		case "query":
			values = append(values, job.Query)
		case "state":
			values = append(values, job.State)
		case "username":
			values = append(values, job.Creator.Username)
		case "createdat":
			values = append(values, job.CreatedAt)
		case "startedat":
			values = append(values, job.StartedAt)
		case "finishedat":
			values = append(values, job.FinishedAt)
		case "url":
			values = append(values, job.URL)
		case "logurl":
			values = append(values, job.LogURL)
		case "total":
			values = append(values, fmt.Sprintf("%d", job.RepoStats.Total))
		case "completed":
			values = append(values, fmt.Sprintf("%d", job.RepoStats.Completed))
		case "failed":
			values = append(values, fmt.Sprintf("%d", job.RepoStats.Failed))
		case "inprogress":
			values = append(values, fmt.Sprintf("%d", job.RepoStats.InProgress))
		}
	}

	fmt.Println(strings.Join(values, "\t"))
	return nil
}

// init registers the 'src search-jobs' command with the CLI. It provides subcommands
// for managing search jobs, including creating, listing, getting, canceling and deleting
// jobs. The command uses a flagset for parsing options and displays usage information
// when help is requested.
func init() {
	usage := `'src search-jobs' is a tool that manages search jobs on a Sourcegraph instance.

	Usage:
	
		src search-jobs command [command options]
	
	The commands are:
	
		cancel     cancels a search job by ID
		create     creates a search job
		delete     deletes a search job by ID
		get        gets a search job by ID
		list       lists search jobs
		logs       fetches logs for a search job by ID
		restart    restarts a search job by ID
		results    fetches results for a search job by ID
	
	Common options for all commands:
		-c          Select columns to display (e.g., -c id,query,state,username)
		-json       Output results in JSON format
	
	Use "src search-jobs [command] -h" for more information about a command.
	`

	flagSet := flag.NewFlagSet("search-jobs", flag.ExitOnError)
	handler := func(args []string) error {
		searchJobsCommands.run(flagSet, "src search-jobs", usage, args)
		return nil
	}

	commands = append(commands, &command{
		flagSet: flagSet,
		aliases: []string{"search-job"},
		handler: handler,
		usageFunc: func() {
			fmt.Println(usage)
		},
	})
}
