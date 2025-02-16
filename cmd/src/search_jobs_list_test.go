package main

import (
    "context"
    "flag"
    "fmt"
    "testing"
    
    "github.com/sourcegraph/src-cli/internal/api"
    "github.com/sourcegraph/src-cli/internal/cmderrors"
    testutil "github.com/sourcegraph/src-cli/internal/testing"
    mockclient "github.com/sourcegraph/src-cli/internal/api/mock"
    "github.com/stretchr/testify/mock"
)
func TestSearchJobsList(t *testing.T) {
    mockClient := new(mockclient.Client)
    mockRequest := new(mockclient.Request)
    
    // Use mock package for expectations
    mockClient.On("NewRequest", mock.Anything, mock.Anything).Return(mockRequest)
}
// Helper function that mimics the actual command execution
func executeSearchJobsList(client api.Client, args []string) error {
    flagSet := flag.NewFlagSet("list", flag.ExitOnError)
    var (
        formatFlag = flagSet.String("f", "{{.ID}}", "")
        limitFlag = flagSet.Int("limit", 10, "")
        ascFlag = flagSet.Bool("asc", false, "")
        orderByFlag = flagSet.String("order-by", "CREATED_AT", "")
    )
    
    if err := flagSet.Parse(args); err != nil {
        return err
    }

    if *limitFlag < 1 {
        return cmderrors.Usage("limit flag must be greater than 0")
    }

    validOrderBy := map[string]bool{
        "QUERY": true,
        "CREATED_AT": true,
        "STATE": true,
    }

    if !validOrderBy[*orderByFlag] {
        return cmderrors.Usage("order-by must be one of: QUERY, CREATED_AT, STATE")
    }

    var result struct {
        SearchJobs struct {
            Nodes []SearchJob
        }
    }

    if ok, err := client.NewRequest(ListSearchJobsQuery, map[string]interface{}{
        "first": *limitFlag,
        "descending": !*ascFlag,
        "orderBy": *orderByFlag,
    }).Do(context.Background(), &result); err != nil || !ok {
        return err
    }

    if len(result.SearchJobs.Nodes) == 0 {
        return cmderrors.ExitCode(1, fmt.Errorf("no search jobs found"))
    }

    return testutil.ExecTemplateWithParsing(*formatFlag, result.SearchJobs.Nodes)
}