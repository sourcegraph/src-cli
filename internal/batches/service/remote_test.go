package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	mockclient "github.com/sourcegraph/src-cli/internal/api/mock"
	"github.com/sourcegraph/src-cli/internal/batches/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

func TestService_UpsertBatchChange(t *testing.T) {
	client := new(mockclient.Client)
	mockRequest := new(mockclient.Request)
	svc := service.New(&service.Opts{Client: client})

	tests := []struct {
		name string

		mockInvokes func()

		requestName        string
		requestNamespaceID string

		expectedID   string
		expectedName string
		expectedErr  error
	}{
		{
			name: "New Batch Change",
			mockInvokes: func() {
				client.On("NewRequest", mock.Anything, map[string]any{
					"name":      "my-change",
					"namespace": "my-namespace",
				}).
					Return(mockRequest, nil).
					Once()
				mockRequest.On("Do", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						json.Unmarshal([]byte(`{"upsertEmptyBatchChange":{"id":"123", "name":"my-change"}}`), &args[1])
					}).
					Return(true, nil).
					Once()
			},
			requestName:        "my-change",
			requestNamespaceID: "my-namespace",
			expectedID:         "123",
			expectedName:       "my-change",
		},
		{
			name: "Failed to upsert batch change",
			mockInvokes: func() {
				client.On("NewRequest", mock.Anything, map[string]any{
					"name":      "my-change",
					"namespace": "my-namespace",
				}).
					Return(mockRequest, nil).
					Once()
				mockRequest.On("Do", mock.Anything, mock.Anything).
					Return(false, errors.New("did not get a good response code")).
					Once()
			},
			requestName:        "my-change",
			requestNamespaceID: "my-namespace",
			expectedErr:        errors.New("did not get a good response code"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.mockInvokes != nil {
				test.mockInvokes()
			}

			id, name, err := svc.UpsertBatchChange(context.Background(), test.requestName, test.requestNamespaceID)
			assert.Equal(t, test.expectedID, id)
			assert.Equal(t, test.expectedName, name)
			if test.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, test.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			client.AssertExpectations(t)
		})
	}
}

func TestService_CreateBatchSpecFromRaw(t *testing.T) {
	client := new(mockclient.Client)
	mockRequest := new(mockclient.Request)
	svc := service.New(&service.Opts{Client: client})

	tests := []struct {
		name string

		mockInvokes func()

		requestBatchSpec        string
		requestNamespaceID      string
		requestAllowIgnored     bool
		requestAllowUnsupported bool
		requestNoCache          bool
		requestBatchChange      string

		expectedID  string
		expectedErr error
	}{
		{
			name: "Create batch spec",
			mockInvokes: func() {
				client.On("NewRequest", mock.Anything, map[string]any{
					"batchSpec":        "abc",
					"namespace":        "some-namespace",
					"allowIgnored":     false,
					"allowUnsupported": false,
					"noCache":          false,
					"batchChange":      "123",
				}).
					Return(mockRequest, nil).
					Once()
				mockRequest.On("Do", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						json.Unmarshal([]byte(`{"createBatchSpecFromRaw":{"id":"xyz"}}`), &args[1])
					}).
					Return(true, nil).
					Once()
			},
			requestBatchSpec:        "abc",
			requestNamespaceID:      "some-namespace",
			requestAllowIgnored:     false,
			requestAllowUnsupported: false,
			requestNoCache:          false,
			requestBatchChange:      "123",
			expectedID:              "xyz",
		},
		{
			name: "Failed to create batch spec",
			mockInvokes: func() {
				client.On("NewRequest", mock.Anything, map[string]any{
					"batchSpec":        "abc",
					"namespace":        "some-namespace",
					"allowIgnored":     false,
					"allowUnsupported": false,
					"noCache":          false,
					"batchChange":      "123",
				}).
					Return(mockRequest, nil).
					Once()
				mockRequest.On("Do", mock.Anything, mock.Anything).
					Return(false, errors.New("did not get a good response code")).
					Once()
			},
			requestBatchSpec:        "abc",
			requestNamespaceID:      "some-namespace",
			requestAllowIgnored:     false,
			requestAllowUnsupported: false,
			requestNoCache:          false,
			requestBatchChange:      "123",
			expectedErr:             errors.New("did not get a good response code"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.mockInvokes != nil {
				test.mockInvokes()
			}

			id, err := svc.CreateBatchSpecFromRaw(
				context.Background(),
				test.requestBatchSpec,
				test.requestNamespaceID,
				test.requestAllowIgnored,
				test.requestAllowUnsupported,
				test.requestNoCache,
				test.requestBatchChange,
			)
			assert.Equal(t, test.expectedID, id)
			if test.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, test.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			client.AssertExpectations(t)
		})
	}
}

func writeTempFile(dir string, name string, content string) error {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = io.WriteString(f, content); err != nil {
		return err
	}
	return nil
}

// 2006-01-02 15:04:05.999999999 -0700 MST
var modtimeRegex = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}\s[0-9]{2}:[0-9]{2}:[0-9]{2}.[0-9]{1,9} \+0000 UTC$`)

func multipartFormRequestMatcher(entry *multipartFormEntry) func(*http.Request) bool {
	return func(req *http.Request) bool {
		// Prevent parsing the body for the wrong matcher - causes all kinds of havoc.
		if entry.calls > 0 {
			return false
		}
		// Clone the request. Running ParseMultipartForm changes the behavior of the request for any additional
		// matchers by consuming the request body.
		cloneReq, err := cloneRequest(req)
		if err != nil {
			fmt.Printf("failed to clone request: %s\n", err)
			return false
		}
		contentType := cloneReq.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "multipart/form-data") {
			return false
		}
		if err := cloneReq.ParseMultipartForm(32 << 20); err != nil {
			fmt.Printf("failed to parse multipartform: %s\n", err)
			return false
		}
		if cloneReq.Form.Get("filepath") != entry.path {
			return false
		}
		if !modtimeRegex.MatchString(cloneReq.Form.Get("filemod")) {
			return false
		}
		f, header, err := cloneReq.FormFile("file")
		if err != nil {
			fmt.Printf("failed to get form file: %s\n", err)
			return false
		}
		if header.Filename != entry.fileName {
			return false
		}
		b, err := io.ReadAll(f)
		if err != nil {
			fmt.Printf("failed to read file: %s\n", err)
			return false
		}
		if string(b) != entry.content {
			return false
		}
		entry.calls++
		return true
	}
}

type multipartFormEntry struct {
	path     string
	fileName string
	content  string
	// This prevents some weird behavior that causes the request body to get read and throw errors.
	calls int
}

type neverEnding byte

func (b neverEnding) Read(p []byte) (n int, err error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}

func cloneRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(context.TODO())
	var b bytes.Buffer
	if _, err := b.ReadFrom(req.Body); err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(&b)
	clone.Body = io.NopCloser(bytes.NewReader(b.Bytes()))
	return clone, nil
}
