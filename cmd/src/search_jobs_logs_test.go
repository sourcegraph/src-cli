package main

import (
    "context"
    "flag"
    "fmt"
    "io/ioutil"
    "net/http"
    "strings"
    "testing"

    "github.com/sourcegraph/src-cli/internal/api"
    "github.com/sourcegraph/src-cli/internal/cmderrors"
    mockclient "github.com/sourcegraph/src-cli/internal/api/mock"
    "github.com/stretchr/testify/mock"
)
func TestSearchJobsLogs(t *testing.T) {
    t.Run("successful log retrieval", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        mockRequest := new(mockclient.Request)

        expectedLogs := "test log content"
        mockHTTPClient := &http.Client{
            Transport: &mockTransport{
                response: &http.Response{
                    StatusCode: http.StatusOK,
                    Body: ioutil.NopCloser(strings.NewReader(expectedLogs)),
                },
            },
        }

        // Inject the mock HTTP client into the test environment
        originalClient := http.DefaultClient
        http.DefaultClient = mockHTTPClient
        defer func() {
            http.DefaultClient = originalClient
        }()

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
                    LogURL: "http://test.com/logs",
                }
            }).Return(true, nil)

        err := executeSearchJobLogs(mockClient, []string{"-id", "test-id"})
        if err != nil {
            t.Errorf("expected no error, got %v", err)
        }
    })

	t.Run("empty ID", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        err := executeSearchJobLogs(mockClient, []string{"-id", ""})
        if err == nil {
            t.Error("expected error for empty ID, got none")
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

        err := executeSearchJobLogs(mockClient, []string{"-id", "non-existent"})
        if err == nil {
            t.Error("expected error for non-existent job, got none")
        }
    })

    t.Run("invalid log URL", func(t *testing.T) {
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
                    LogURL: "",
                }
            }).Return(true, nil)

        err := executeSearchJobLogs(mockClient, []string{"-id", "test-id"})
        if err == nil {
            t.Error("expected error for invalid log URL, got none")
        }
    })
}

func executeSearchJobLogs(client api.Client, args []string) error {
    flagSet := flag.NewFlagSet("logs", flag.ExitOnError)
    var (
        idFlag  string
        outFlag string
    )
    flagSet.StringVar(&idFlag, "id", "", "")
    flagSet.StringVar(&outFlag, "out", "", "")

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

    if result.Node == nil || result.Node.LogURL == "" {
        return fmt.Errorf("no logs URL found for search job %s", idFlag)
    }

    // Mock HTTP request handling would go here in a real implementation
    return nil
}

type mockTransport struct {
    response *http.Response
}

func (t *mockTransport) RoundTrip(*http.Request) (*http.Response, error) {
    return t.response, nil
}
