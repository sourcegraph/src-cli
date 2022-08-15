package service_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestService_UploadMounts(t *testing.T) {
	client := new(mockclient.Client)

	// Mock SG version to enable ServerSideBatchChanges
	versionReq := new(mockclient.Request)
	versionReq.Response = `{"Site":{"ProductVersion":"3.42.0-0"}}`
	client.On("NewQuery", mock.Anything).Return(versionReq)
	versionReq.On("Do", mock.Anything, mock.Anything).Return(true, nil)

	svc := service.New(&service.Opts{Client: client})

	err := svc.DetermineFeatureFlags(context.Background())
	require.NoError(t, err)

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
				if err = writeTempFile(workingDir, "world.txt", "world!"); err != nil {
					return err
				}
				dir := filepath.Join(workingDir, "scripts")
				if err = os.Mkdir(dir, os.ModePerm); err != nil {
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
				err = test.setup()
				require.NoError(t, err)
			}

			if test.mockInvokes != nil {
				test.mockInvokes()
			}

			err = svc.UploadMounts(workingDir, "123", test.steps)
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
