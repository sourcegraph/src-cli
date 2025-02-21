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

func TestSearchJobsGet(t *testing.T) {
    t.Run("existing job", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        mockRequest := new(mockclient.Request)

        mockClient.On("NewRequest",
            GetSearchJobQuery + SearchJobFragment,
            map[string]interface{}{"id": "test-id"},
        ).Return(mockRequest)

        mockRequest.On("Do", mock.Anything, mock.Anything).
            Run(func(args mock.Arguments) {
                result := args.Get(1).(*struct {
                    Node *SearchJob
                })
                result.Node = &SearchJob{
                    ID: "test-id",
                    Query: "repo:test",
                    State: "COMPLETED",
                    Creator: struct{ Username string }{Username: "test-user"},
                }
            }).Return(true, nil)

        err := executeSearchJobGet(mockClient, []string{"-id", "test-id"})
        if err != nil {
            t.Errorf("expected no error, got %v", err)
        }
    })

    t.Run("non-existent job", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        mockRequest := new(mockclient.Request)

        mockClient.On("NewRequest", mock.Anything, mock.Anything).Return(mockRequest)
        mockRequest.On("Do", mock.Anything, mock.Anything).
            Run(func(args mock.Arguments) {
                result := args.Get(1).(*struct {
                    Node *SearchJob
                })
                result.Node = nil
            }).Return(true, nil)

        err := executeSearchJobGet(mockClient, []string{"-id", "non-existent"})
        if err == nil {
            t.Error("expected error for non-existent job, got none")
        }
    })

    t.Run("empty ID", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        err := executeSearchJobGet(mockClient, []string{"-id", ""})
        if err == nil {
            t.Error("expected error for empty ID, got none")
        }
    })

    t.Run("output formatting", func(t *testing.T) {
        formats := []string{
            "{{.ID}}",
            "{{.Query}}",
            "{{.State}}",
            "{{.Creator.Username}}",
            "{{.|json}}",
        }

        for _, format := range formats {
            mockClient := new(mockclient.Client)
            mockRequest := new(mockclient.Request)

            mockClient.On("NewRequest", mock.Anything, mock.Anything).Return(mockRequest)
            mockRequest.On("Do", mock.Anything, mock.Anything).
                Run(func(args mock.Arguments) {
                    result := args.Get(1).(*struct {
                        Node *SearchJob
                    })
                    result.Node = &SearchJob{
                        ID: "test-id",
                        Query: "repo:test",
                        State: "COMPLETED",
                        Creator: struct{ Username string }{Username: "test-user"},
                    }
                }).Return(true, nil)

            err := executeSearchJobGet(mockClient, []string{"-id", "test-id", "-f", format})
            if err != nil {
                t.Errorf("format %q failed: %v", format, err)
            }
        }
    })

    t.Run("invalid ID format", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        mockRequest := new(mockclient.Request)

        mockClient.On("NewRequest", mock.Anything, mock.Anything).Return(mockRequest)
        mockRequest.On("Do", mock.Anything, mock.Anything).Return(false, fmt.Errorf("invalid ID format"))

        err := executeSearchJobGet(mockClient, []string{"-id", "invalid-format"})
        if err == nil {
            t.Error("expected error for invalid ID format, got none")
        }
    })
}

func executeSearchJobGet(client api.Client, args []string) error {
    flagSet := flag.NewFlagSet("get", flag.ExitOnError)
    var (
        idFlag     string
        formatFlag string
    )
    flagSet.StringVar(&idFlag, "id", "", "")
    flagSet.StringVar(&formatFlag, "f", "{{.ID}}", "")

    if err := flagSet.Parse(args); err != nil {
        return err
    }

    if idFlag == "" {
        return cmderrors.Usage("must provide a search job ID")
    }

    var result struct {
        Node *SearchJob
    }

    if ok, err := client.NewRequest(GetSearchJobQuery + SearchJobFragment, map[string]interface{}{
        "id": idFlag,
    }).Do(context.Background(), &result); err != nil || !ok {
        return err
    }

    if result.Node == nil {
        return fmt.Errorf("search job not found: %q", idFlag)
    }

    return testutil.ExecTemplateWithParsing(formatFlag, result.Node)
}
