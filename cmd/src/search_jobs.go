package main

import (
	"flag"
	"fmt"
	"text/template"
	"os"
)

// searchJobFragment is a GraphQL fragment that defines the fields to be queried
// for a SearchJob. It includes the job's ID, query, state, creator information,
// timestamps, URLs, and repository statistics.
const SearchJobFragment = `
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

var searchJobsCommands commander

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

// printSearchJob formats and prints a search job to stdout using the provided format template.
// Returns an error if the template parsing or execution fails.
func printSearchJob(job *SearchJob, format string) error {
    tmpl, err := template.New("searchJob").Parse(format)
    if err != nil {
        return err
    }
    return tmpl.Execute(os.Stdout, job)
}

// SearchJob represents a search job with its metadata, including the search query,
// execution state, creator information, timestamps, URLs, and repository statistics.
type SearchJob struct {
	ID        string
	Query     string
	State     string
	Creator struct {
		Username string
	}
	CreatedAt  string
	StartedAt  string
	FinishedAt string
	URL        string
	LogURL     string
	RepoStats  struct {
		Total       int
		Completed   int
		Failed      int
		InProgress  int
	}
}
