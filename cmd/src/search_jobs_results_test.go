package main

import (
    "context"
    "flag"
    "io/ioutil"
    "net/http"
    "strings"
    "testing"
	"fmt"

    "github.com/sourcegraph/src-cli/internal/api"
    "github.com/sourcegraph/src-cli/internal/cmderrors"
    mockclient "github.com/sourcegraph/src-cli/internal/api/mock"
    "github.com/stretchr/testify/mock"
)

func TestSearchJobsResults(t *testing.T) {
    t.Run("successful results retrieval", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        mockRequest := new(mockclient.Request)
        
        expectedResults := `{"result": "test search results"}`
        mockHTTPClient := &http.Client{
            Transport: &mockTransport{
                response: &http.Response{
                    StatusCode: http.StatusOK,
                    Body: ioutil.NopCloser(strings.NewReader(expectedResults)),
                },
            },
        }

        // Inject mock HTTP client
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
                    URL: "http://test.com/results",
                }
            }).Return(true, nil)

        err := executeSearchJobResults(mockClient, []string{"-id", "test-id"})
        if err != nil {
            t.Errorf("expected no error, got %v", err)
        }
    })

    t.Run("empty ID", func(t *testing.T) {
        mockClient := new(mockclient.Client)
        err := executeSearchJobResults(mockClient, []string{"-id", ""})
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

        err := executeSearchJobResults(mockClient, []string{"-id", "non-existent"})
        if err == nil {
            t.Error("expected error for non-existent job, got none")
        }
    })

    t.Run("invalid results URL", func(t *testing.T) {
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
                    URL: "",
                }
            }).Return(true, nil)

        err := executeSearchJobResults(mockClient, []string{"-id", "test-id"})
        if err == nil {
            t.Error("expected error for invalid results URL, got none")
        }
    })
}

func executeSearchJobResults(client api.Client, args []string) error {
    flagSet := flag.NewFlagSet("results", flag.ExitOnError)
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

    if result.Node == nil || result.Node.URL == "" {
        return fmt.Errorf("no results URL found for search job %s", idFlag)
    }

    return nil
}
