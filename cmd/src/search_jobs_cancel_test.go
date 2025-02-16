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

func TestSearchJobsCancel(t *testing.T) {
    t.Run("running job", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        mockRequest := new(mockclient.Request)

        mockClient.On("NewRequest", 
            CancelSearchJobMutation,
            map[string]interface{}{"id": "test-id"},
        ).Return(mockRequest)

        mockRequest.On("Do", mock.Anything, mock.Anything).
            Run(func(args mock.Arguments) {
                result := args.Get(1).(*struct {
                    CancelSearchJob struct {
                        AlwaysNil interface{}
                    }
                })
                result.CancelSearchJob.AlwaysNil = nil
            }).Return(true, nil)

        err := executeSearchJobCancel(mockClient, []string{"-id", "test-id"})
        if err != nil {
            t.Errorf("expected no error, got %v", err)
        }
    })

    t.Run("completed job", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        mockRequest := new(mockclient.Request)

        mockClient.On("NewRequest", mock.Anything, mock.Anything).Return(mockRequest)
        mockRequest.On("Do", mock.Anything, mock.Anything).Return(false, fmt.Errorf("cannot cancel completed job"))

        err := executeSearchJobCancel(mockClient, []string{"-id", "completed-id"})
        if err == nil {
            t.Error("expected error for completed job, got none")
        }
    })

    t.Run("non-existent job", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        mockRequest := new(mockclient.Request)

        mockClient.On("NewRequest", mock.Anything, mock.Anything).Return(mockRequest)
        mockRequest.On("Do", mock.Anything, mock.Anything).Return(false, fmt.Errorf("job not found"))

        err := executeSearchJobCancel(mockClient, []string{"-id", "non-existent"})
        if err == nil {
            t.Error("expected error for non-existent job, got none")
        }
    })

    t.Run("empty ID", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        err := executeSearchJobCancel(mockClient, []string{"-id", ""})
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

                err := executeSearchJobCancel(mockClient, []string{"-id", tc.id})
                if err == nil {
                    t.Errorf("expected error for %s, got none", tc.name)
                }
            })
        }
    })
}

func executeSearchJobCancel(client api.Client, args []string) error {
    flagSet := flag.NewFlagSet("cancel", flag.ExitOnError)
    var idFlag string
    flagSet.StringVar(&idFlag, "id", "", "")
    
    if err := flagSet.Parse(args); err != nil {
        return err
    }

    if idFlag == "" {
        return cmderrors.Usage("must provide a search job ID")
    }

    var result struct {
        CancelSearchJob struct {
            AlwaysNil interface{}
        }
    }

    if ok, err := client.NewRequest(CancelSearchJobMutation, map[string]interface{}{
        "id": idFlag,
    }).Do(context.Background(), &result); err != nil || !ok {
        return err
    }

    fmt.Printf("Search job %s canceled successfully\n", idFlag)
    return nil
}
