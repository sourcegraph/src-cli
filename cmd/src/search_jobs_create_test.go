package main

import (
    "context"
    "flag"
    "testing"

    "github.com/sourcegraph/src-cli/internal/api"
    "github.com/sourcegraph/src-cli/internal/cmderrors"
    testutil "github.com/sourcegraph/src-cli/internal/testing"
    mockclient "github.com/sourcegraph/src-cli/internal/api/mock"
    "github.com/stretchr/testify/mock"
)
func TestSearchJobsCreate(t *testing.T) {
    t.Run("valid query", func(t *testing.T) {
        mockClient := new(mockclient.Client)

        // Set up validation request
        validationRequest := new(mockclient.Request)
        mockClient.On("NewRequest",
            ValidateSearchJobQuery,
            map[string]interface{}{"query": "repo:test"},
        ).Return(validationRequest)

        validationRequest.On("Do", mock.Anything, mock.Anything).Return(true, nil)

        // Set up creation request
        creationRequest := new(mockclient.Request)
        mockClient.On("NewRequest",
            CreateSearchJobQuery,
            map[string]interface{}{"query": "repo:test"},
        ).Return(creationRequest)

        creationRequest.On("Do", mock.Anything, mock.Anything).
            Run(func(args mock.Arguments) {
                result := args.Get(1).(*struct {
                    CreateSearchJob *SearchJob `json:"createSearchJob"`
                })
                result.CreateSearchJob = &SearchJob{
                    ID: "test-id",
                    Query: "repo:test",
                    State: "QUEUED",
                    Creator: struct{ Username string }{Username: "test-user"},
                }
            }).Return(true, nil)

        err := executeSearchJobCreate(mockClient, []string{"-query", "repo:test"})
        if err != nil {
            t.Errorf("expected no error, got %v", err)
        }
    })
}

func executeSearchJobCreate(client api.Client, args []string) error {
    flagSet := flag.NewFlagSet("create", flag.ExitOnError)
    var (
        queryFlag  string
        formatFlag string
    )
    flagSet.StringVar(&queryFlag, "query", "", "")
    flagSet.StringVar(&formatFlag, "f", "{{.ID}}", "")

    if err := flagSet.Parse(args); err != nil {
        return err
    }

    if queryFlag == "" {
        return cmderrors.Usage("must provide a query")
    }

    // Define result structure before using it
    var result struct {
        CreateSearchJob *SearchJob `json:"createSearchJob"`
    }

    if ok, err := client.NewRequest(CreateSearchJobQuery, map[string]interface{}{
        "query": queryFlag,
    }).Do(context.Background(), &result); !ok {
        return err
    }

    // Now we can use result with proper scoping
    return testutil.ExecTemplateWithParsing(formatFlag, result.CreateSearchJob)
}