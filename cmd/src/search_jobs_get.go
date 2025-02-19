package main

import (
    "context"
    "flag"
    "fmt"
    "github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

const GetSearchJobQuery = `query SearchJob($id: ID!) {
    node(id: $id) {
        ... on SearchJob {
            ...SearchJobFields
        }
    }
}
` 

// init registers the "get" subcommand for search-jobs which retrieves a search job by ID.
// It supports formatting the output using Go templates and requires authentication via API flags.
func init() {
    usage := `
Examples:

  Get a search job by ID:
  
    $ src search-jobs get U2VhcmNoSm9iOjY5
`

    flagSet := flag.NewFlagSet("get", flag.ExitOnError)
    usageFunc := func() {
        fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src search-jobs %s':\n", flagSet.Name())
        flagSet.PrintDefaults()
        fmt.Println(usage)
    }
    
    var (
		idFlag = flagSet.String("id", "", "ID of the search job")
        formatFlag = flagSet.String("f", "{{.ID}}: {{.Creator.Username}} {{.State}} ({{.Query}})", `Format for the output, using the syntax of Go package text/template. (e.g. "{{.ID}}: {{.Creator.Username}} ({{.Query}})" or "{{.|json}}")`)
        apiFlags   = api.NewFlags(flagSet)
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

        tmpl, err := parseTemplate(*formatFlag)
        if err != nil {
            return err
        }

        if *idFlag == "" {
            return cmderrors.Usage("must provide a search job ID")
        }

        job, err := getSearchJob(client, *idFlag)
        if err != nil {
            return err
        }
        return execTemplate(tmpl, job)
    }
    searchJobsCommands = append(searchJobsCommands, &command{
        flagSet:   flagSet,
        handler:   handler,
        usageFunc: usageFunc,
    })
}

func getSearchJob(client api.Client, id string) (*SearchJob, error) {
    query := GetSearchJobQuery + SearchJobFragment
    
    var result struct {
        Node *SearchJob
    }
    
    if ok, err := client.NewRequest(query, map[string]interface{}{
        "id": api.NullString(id),
    }).Do(context.Background(), &result); err != nil || !ok {
        return nil, err
    }
    
    return result.Node, nil
}
