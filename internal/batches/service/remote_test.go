package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	mockclient "github.com/sourcegraph/src-cli/internal/api/mock"
	"github.com/sourcegraph/src-cli/internal/batches/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sourcegraph/sourcegraph/lib/batches"
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
				client.On("NewRequest", mock.Anything, map[string]interface{}{
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
				client.On("NewRequest", mock.Anything, map[string]interface{}{
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
				client.On("NewRequest", mock.Anything, map[string]interface{}{
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
				client.On("NewRequest", mock.Anything, map[string]interface{}{
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

func TestService_UploadMounts(t *testing.T) {
	client := new(mockclient.Client)
	svc := service.New(&service.Opts{Client: client})

	// Use a temp directory for reading files
	workingDir := t.TempDir()

	tests := []struct {
		name  string
		steps []batches.Step

		setup       func() error
		mockInvokes func()

		expectedError error
	}{
		{
			name: "Upload single file",
			steps: []batches.Step{{
				Mount: []batches.Mount{{
					Path: "./hello.txt",
				}},
			}},
			setup: func() error {
				return writeTempFile(workingDir, "hello.txt", "hello world!")
			},
			mockInvokes: func() {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/batches/mount/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/batches/mount/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = io.NopCloser(args.Get(3).(*bytes.Buffer))
					}).
					Return(req, nil).
					Once()
				resp := &http.Response{
					StatusCode: http.StatusOK,
				}
				requestMatcher := multipartFormRequestMatcher(
					multipartFormEntry{
						fileName: "hello.txt",
						content:  "hello world!",
					},
				)
				client.On("Do", mock.MatchedBy(requestMatcher)).
					Return(resp, nil).
					Once()
			},
		},
		{
			name: "Upload multiple files",
			steps: []batches.Step{{
				Mount: []batches.Mount{
					{
						Path: "./hello.txt",
					},
					{
						Path: "./world.txt",
					},
				},
			}},
			setup: func() error {
				if err := writeTempFile(workingDir, "hello.txt", "hello"); err != nil {
					return err
				}
				return writeTempFile(workingDir, "world.txt", "world!")
			},
			mockInvokes: func() {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/batches/mount/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/batches/mount/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = io.NopCloser(args.Get(3).(*bytes.Buffer))
					}).
					Return(req, nil).
					Once()
				resp := &http.Response{
					StatusCode: http.StatusOK,
				}
				requestMatcher := multipartFormRequestMatcher(
					multipartFormEntry{
						fileName: "hello.txt",
						content:  "hello",
					},
					multipartFormEntry{
						fileName: "world.txt",
						content:  "world!",
					},
				)
				client.On("Do", mock.MatchedBy(requestMatcher)).
					Return(resp, nil).
					Once()
			},
		},
		{
			name: "Upload directory",
			steps: []batches.Step{{
				Mount: []batches.Mount{
					{
						Path: "./",
					},
				},
			}},
			setup: func() error {
				if err := writeTempFile(workingDir, "hello.txt", "hello"); err != nil {
					return err
				}
				return writeTempFile(workingDir, "world.txt", "world!")
			},
			mockInvokes: func() {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/batches/mount/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/batches/mount/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = io.NopCloser(args.Get(3).(*bytes.Buffer))
					}).
					Return(req, nil).
					Once()
				resp := &http.Response{
					StatusCode: http.StatusOK,
				}
				requestMatcher := multipartFormRequestMatcher(
					multipartFormEntry{
						fileName: "hello.txt",
						content:  "hello",
					},
					multipartFormEntry{
						fileName: "world.txt",
						content:  "world!",
					},
				)
				client.On("Do", mock.MatchedBy(requestMatcher)).
					Return(resp, nil).
					Once()
			},
		},
		{
			name: "Upload subdirectory",
			steps: []batches.Step{{
				Mount: []batches.Mount{
					{
						Path: "./scripts",
					},
				},
			}},
			setup: func() error {
				dir := filepath.Join(workingDir, "scripts")
				if err := os.Mkdir(dir, os.ModePerm); err != nil {
					return err
				}
				return writeTempFile(dir, "hello.txt", "hello world!")
			},
			mockInvokes: func() {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/batches/mount/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/batches/mount/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = io.NopCloser(args.Get(3).(*bytes.Buffer))
					}).
					Return(req, nil).
					Once()
				resp := &http.Response{
					StatusCode: http.StatusOK,
				}
				requestMatcher := multipartFormRequestMatcher(
					multipartFormEntry{
						path:     "scripts",
						fileName: "hello.txt",
						content:  "hello world!",
					},
				)
				client.On("Do", mock.MatchedBy(requestMatcher)).
					Return(resp, nil).
					Once()
			},
		},
		{
			name: "Upload files and directory",
			steps: []batches.Step{{
				Mount: []batches.Mount{
					{
						Path: "./hello.txt",
					},
					{
						Path: "./world.txt",
					},
					{
						Path: "./scripts",
					},
				},
			}},
			setup: func() error {
				if err := writeTempFile(workingDir, "hello.txt", "hello"); err != nil {
					return err
				}
				if err := writeTempFile(workingDir, "world.txt", "world!"); err != nil {
					return err
				}
				dir := filepath.Join(workingDir, "scripts")
				if err := os.Mkdir(dir, os.ModePerm); err != nil {
					return err
				}
				return writeTempFile(dir, "something-else.txt", "this is neat")
			},
			mockInvokes: func() {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/batches/mount/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/batches/mount/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = io.NopCloser(args.Get(3).(*bytes.Buffer))
					}).
					Return(req, nil).
					Once()
				resp := &http.Response{
					StatusCode: http.StatusOK,
				}
				requestMatcher := multipartFormRequestMatcher(
					multipartFormEntry{
						fileName: "hello.txt",
						content:  "hello",
					},
					multipartFormEntry{
						fileName: "world.txt",
						content:  "world!",
					},
					multipartFormEntry{
						path:     "scripts",
						fileName: "something-else.txt",
						content:  "this is neat",
					},
				)
				client.On("Do", mock.MatchedBy(requestMatcher)).
					Return(resp, nil).
					Once()
			},
		},
		{
			name: "File does not exist",
			steps: []batches.Step{{
				Mount: []batches.Mount{{
					Path: "./this-does-not-exist.txt",
				}},
			}},
			expectedError: errors.Newf("stat %s/this-does-not-exist.txt: no such file or directory", workingDir),
		},
		{
			name: "Bad status code",
			steps: []batches.Step{{
				Mount: []batches.Mount{{
					Path: "./hello.txt",
				}},
			}},
			setup: func() error {
				return writeTempFile(workingDir, "hello.txt", "hello world!")
			},
			mockInvokes: func() {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/batches/mount/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/batches/mount/123", mock.Anything).
					Return(req, nil).
					Once()
				resp := &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewReader([]byte("failed to upload file"))),
				}
				client.On("Do", mock.Anything).
					Return(resp, nil).
					Once()
			},
			expectedError: errors.New("failed to upload file"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				err := test.setup()
				require.NoError(t, err)
			}

			if test.mockInvokes != nil {
				test.mockInvokes()
			}

			err := svc.UploadMounts(workingDir, "123", test.steps)
			if test.expectedError != nil {
				assert.Equal(t, test.expectedError.Error(), err.Error())
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
var modtimeRegex = regexp.MustCompile("^[0-9]{4}-[0-9]{2}-[0-9]{2}\\s[0-9]{2}:[0-9]{2}:[0-9]{2}.[0-9]{9} \\+0000 UTC$")

func multipartFormRequestMatcher(entries ...multipartFormEntry) func(*http.Request) bool {
	return func(req *http.Request) bool {
		contentType := req.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "multipart/form-data") {
			return false
		}
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			return false
		}
		if req.Form.Get("count") != strconv.Itoa(len(entries)) {
			return false
		}
		for i, entry := range entries {
			if req.Form.Get(fmt.Sprintf("filepath_%d", i)) != entry.path {
				return false
			}
			if !modtimeRegex.MatchString(req.Form.Get(fmt.Sprintf("filemod_%d", i))) {
				return false
			}
			f, header, err := req.FormFile(fmt.Sprintf("file_%d", i))
			if err != nil {
				return false
			}
			if header.Filename != entry.fileName {
				return false
			}
			b, err := io.ReadAll(f)
			if err != nil {
				return false
			}
			if string(b) != entry.content {
				return false
			}
		}
		return true
	}
}

type multipartFormEntry struct {
	path     string
	fileName string
	content  string
}
