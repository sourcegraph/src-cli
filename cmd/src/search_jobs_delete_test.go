package main

import (
    "context"
    "flag"
    "fmt"
    "testing"
    
    "github.com/sourcegraph/src-cli/internal/api"
    "github.com/sourcegraph/src-cli/internal/cmderrors"
    mockclient "github.com/sourcegraph/src-cli/internal/api/mock"
    "github.com/stretchr/testify/mock"
)
func TestSearchJobsDelete(t *testing.T) {
    t.Run("existing job", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        mockRequest := new(mockclient.Request)

        mockClient.On("NewRequest", 
            DeleteSearchJobQuery,
            map[string]interface{}{"id": "test-id"},
        ).Return(mockRequest)

        mockRequest.On("Do", mock.Anything, mock.Anything).
            Run(func(args mock.Arguments) {
                result := args.Get(1).(*struct {
                    DeleteSearchJob struct {
                        AlwaysNil interface{}
                    }
                })
                // Simulate successful deletion
                result.DeleteSearchJob.AlwaysNil = nil
            }).Return(true, nil)

        err := executeSearchJobDelete(mockClient, []string{"-id", "test-id"})
        if err != nil {
            t.Errorf("expected no error, got %v", err)
        }
    })

    t.Run("non-existent job", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        mockRequest := new(mockclient.Request)

        mockClient.On("NewRequest", mock.Anything, mock.Anything).Return(mockRequest)
        mockRequest.On("Do", mock.Anything, mock.Anything).Return(false, fmt.Errorf("job not found"))

        err := executeSearchJobDelete(mockClient, []string{"-id", "non-existent"})
        if err == nil {
            t.Error("expected error for non-existent job, got none")
        }
    })

    t.Run("empty ID", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        err := executeSearchJobDelete(mockClient, []string{"-id", ""})
        if err == nil {
            t.Error("expected error for empty ID, got none")
        }
    })

    t.Run("error handling", func(t *testing.T) {
        testCases := []struct {
            name    string
            id      string
            mockErr error
        }{
            {"network error", "test-id", fmt.Errorf("network error")},
            {"server error", "test-id", fmt.Errorf("internal server error")},
            {"invalid ID", "invalid-id", fmt.Errorf("invalid ID format")},
        }

        for _, tc := range testCases {
            t.Run(tc.name, func(t *testing.T) {
                mockClient := new(mockclient.Client)
                mockRequest := new(mockclient.Request)

                mockClient.On("NewRequest", mock.Anything, mock.Anything).Return(mockRequest)
                mockRequest.On("Do", mock.Anything, mock.Anything).Return(false, tc.mockErr)

                err := executeSearchJobDelete(mockClient, []string{"-id", tc.id})
                if err == nil {
                    t.Errorf("expected error for %s, got none", tc.name)
                }
            })
        }
    })
}

func executeSearchJobDelete(client api.Client, args []string) error {
    flagSet := flag.NewFlagSet("delete", flag.ExitOnError)
    var idFlag string
    flagSet.StringVar(&idFlag, "id", "", "")
    
    if err := flagSet.Parse(args); err != nil {
        return err
    }

    if idFlag == "" {
        return cmderrors.Usage("must provide a search job ID")
    }

    var result struct {
        DeleteSearchJob struct {
            AlwaysNil interface{}
        }
    }

    if ok, err := client.NewRequest(DeleteSearchJobQuery, map[string]interface{}{
        "id": idFlag,
    }).Do(context.Background(), &result); err != nil || !ok {
        return err
    }

    fmt.Printf("Search job %s deleted successfully\n", idFlag)
    return nil
}