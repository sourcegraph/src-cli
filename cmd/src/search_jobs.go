package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"regexp"
	"strconv"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
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
	restart    restarts a search job by ID
	list       lists search jobs
	logs       outputs the logs for a search job by ID
	results    outputs the results for a search job by ID

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

type SearchJobID struct {
	number uint64
}

func ParseSearchJobID(input string) (*SearchJobID, error) {
	// accept either:
	// - the numeric job id (non-negative integer)
	// - the plain text SearchJob:<integer> form of the id
	// - the base64-encoded "SearchJob:<integer>" string

	if input == "" {
		return nil, cmderrors.Usage("must provide a search job ID")
	}

	// Try to decode if it's base64 first
	if decoded, err := base64.StdEncoding.DecodeString(input); err == nil {
		input = string(decoded)
	}

	// Match either "SearchJob:<integer>" or "<integer>"
	re := regexp.MustCompile(`^(?:SearchJob:)?(\d+)$`)
	matches := re.FindStringSubmatch(input)
	if matches == nil {
		return nil, fmt.Errorf("invalid ID format: must be a non-negative integer, 'SearchJob:<integer>', or that string base64-encoded")
	}

	number, err := strconv.ParseUint(matches[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid ID format: must be a 64-bit non-negative integer")
	}

	return &SearchJobID{number: number}, nil
}

func (id *SearchJobID) String() string {
	return fmt.Sprintf("SearchJob:%d", id.Number())
}

func (id *SearchJobID) Canonical() string {
	return base64.StdEncoding.EncodeToString([]byte(id.String()))
}

func (id *SearchJobID) Number() uint64 {
	return id.number
}
