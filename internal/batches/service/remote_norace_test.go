//go:build !race

package service

import (
	"bytes"
	"context"
	"github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	mockclient "github.com/sourcegraph/src-cli/internal/api/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

func TestService_UploadBatchSpecWorkspaceFiles(t *testing.T) {
	tests := []struct {
		name  string
		steps []batches.Step

		setup       func(workingDir string) error
		mockInvokes func(client *mockclient.Client)

		expectedError error
	}{
		{
			name: "Upload single file",
			steps: []batches.Step{{
				Mount: []batches.Mount{{
					Path: "./hello.txt",
				}},
			}},
			setup: func(workingDir string) error {
				return writeTempFile(workingDir, "hello.txt", "hello world!")
			},
			mockInvokes: func(client *mockclient.Client) {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/files/batch-changes/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/files/batch-changes/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = args[3].(*io.PipeReader)
					}).
					Return(req, nil).
					Once()

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte{})),
				}
				entry := &multipartFormEntry{
					fileName: "hello.txt",
					content:  "hello world!",
				}
				requestMatcher := multipartFormRequestMatcher(entry)
				client.On("Do", mock.MatchedBy(requestMatcher)).
					Return(resp, nil).
					Once()
			},
		},
		{
			name: "Deduplicate files",
			steps: []batches.Step{{
				Mount: []batches.Mount{
					{
						Path: "./hello.txt",
					},
					{
						Path: "./hello.txt",
					},
					{
						Path: "./hello.txt",
					},
				},
			}},
			setup: func(workingDir string) error {
				return writeTempFile(workingDir, "hello.txt", "hello world!")
			},
			mockInvokes: func(client *mockclient.Client) {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/files/batch-changes/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/files/batch-changes/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = args[3].(*io.PipeReader)
					}).
					Return(req, nil).
					Once()

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte{})),
				}
				entry := &multipartFormEntry{
					fileName: "hello.txt",
					content:  "hello world!",
				}
				requestMatcher := multipartFormRequestMatcher(entry)
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
			setup: func(workingDir string) error {
				if err := writeTempFile(workingDir, "hello.txt", "hello"); err != nil {
					return err
				}
				return writeTempFile(workingDir, "world.txt", "world!")
			},
			mockInvokes: func(client *mockclient.Client) {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/files/batch-changes/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/files/batch-changes/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = args[3].(*io.PipeReader)
					}).
					Return(req, nil).
					Twice()

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte{})),
				}
				helloEntry := &multipartFormEntry{
					fileName: "hello.txt",
					content:  "hello",
				}
				client.
					On("Do", mock.MatchedBy(multipartFormRequestMatcher(helloEntry))).
					Return(resp, nil).
					Once()

				worldEntry := &multipartFormEntry{
					fileName: "world.txt",
					content:  "world!",
				}
				client.
					On("Do", mock.MatchedBy(multipartFormRequestMatcher(worldEntry))).
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
			setup: func(workingDir string) error {
				if err := writeTempFile(workingDir, "hello.txt", "hello"); err != nil {
					return err
				}
				return writeTempFile(workingDir, "world.txt", "world!")
			},
			mockInvokes: func(client *mockclient.Client) {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/files/batch-changes/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/files/batch-changes/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = args[3].(*io.PipeReader)
					}).
					Return(req, nil).
					Twice()

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte{})),
				}
				helloEntry := &multipartFormEntry{
					fileName: "hello.txt",
					content:  "hello",
				}
				client.
					On("Do", mock.MatchedBy(multipartFormRequestMatcher(helloEntry))).
					Return(resp, nil).
					Once()

				worldEntry := &multipartFormEntry{
					fileName: "world.txt",
					content:  "world!",
				}
				client.
					On("Do", mock.MatchedBy(multipartFormRequestMatcher(worldEntry))).
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
			setup: func(workingDir string) error {
				dir := filepath.Join(workingDir, "scripts")
				if err := os.Mkdir(dir, os.ModePerm); err != nil {
					return err
				}
				return writeTempFile(dir, "hello.txt", "hello world!")
			},
			mockInvokes: func(client *mockclient.Client) {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/files/batch-changes/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/files/batch-changes/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = args[3].(*io.PipeReader)
					}).
					Return(req, nil).
					Once()

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte{})),
				}
				entry := &multipartFormEntry{
					path:     "scripts",
					fileName: "hello.txt",
					content:  "hello world!",
				}
				client.On("Do", mock.MatchedBy(multipartFormRequestMatcher(entry))).
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
			setup: func(workingDir string) error {
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
			mockInvokes: func(client *mockclient.Client) {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/files/batch-changes/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/files/batch-changes/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = args[3].(*io.PipeReader)
					}).
					Return(req, nil).
					Times(3)

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte{})),
				}
				helloEntry := &multipartFormEntry{
					fileName: "hello.txt",
					content:  "hello",
				}
				client.On("Do", mock.MatchedBy(multipartFormRequestMatcher(helloEntry))).
					Return(resp, nil).
					Once()
				worldEntry := &multipartFormEntry{
					fileName: "world.txt",
					content:  "world!",
				}
				client.On("Do", mock.MatchedBy(multipartFormRequestMatcher(worldEntry))).
					Return(resp, nil).
					Once()
				somethingElseEntry := &multipartFormEntry{
					path:     "scripts",
					fileName: "something-else.txt",
					content:  "this is neat",
				}
				client.On("Do", mock.MatchedBy(multipartFormRequestMatcher(somethingElseEntry))).
					Return(resp, nil).
					Once()
			},
		},
		{
			name: "Bad status code",
			steps: []batches.Step{{
				Mount: []batches.Mount{{
					Path: "./hello.txt",
				}},
			}},
			setup: func(workingDir string) error {
				return writeTempFile(workingDir, "hello.txt", "hello world!")
			},
			mockInvokes: func(client *mockclient.Client) {
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/files/batch-changes/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/files/batch-changes/123", mock.Anything).
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
		{
			name: "File exceeds limit",
			steps: []batches.Step{{
				Mount: []batches.Mount{{
					Path: "./hello.txt",
				}},
			}},
			setup: func(workingDir string) error {
				f, err := os.Create(filepath.Join(workingDir, "hello.txt"))
				if err != nil {
					return err
				}
				defer f.Close()
				if _, err = io.Copy(f, io.LimitReader(neverEnding('a'), 11<<20)); err != nil {
					return err
				}
				return nil
			},
			mockInvokes: func(client *mockclient.Client) {
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/files/batch-changes/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/files/batch-changes/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = args[3].(*io.PipeReader)
					}).
					Return(req, nil).
					Once()

				client.On("Do", mock.Anything).
					Return(nil, errors.New("file exceeds limit")).
					Once()
			},
			expectedError: errors.New("file exceeds limit"),
		},
		{
			name: "Long mount path",
			steps: []batches.Step{{
				Mount: []batches.Mount{{
					Path: "foo/../bar/../baz/../hello.txt",
				}},
			}},
			setup: func(workingDir string) error {
				dir := filepath.Join(workingDir, "foo")
				if err := os.Mkdir(dir, os.ModePerm); err != nil {
					return err
				}
				dir = filepath.Join(workingDir, "bar")
				if err := os.Mkdir(dir, os.ModePerm); err != nil {
					return err
				}
				dir = filepath.Join(workingDir, "baz")
				if err := os.Mkdir(dir, os.ModePerm); err != nil {
					return err
				}
				return writeTempFile(workingDir, "hello.txt", "hello world!")
			},
			mockInvokes: func(client *mockclient.Client) {
				// Body will get set with the body argument to NewHTTPRequest
				req := httptest.NewRequest(http.MethodPost, "http://fake.com/.api/files/batch-changes/123", nil)
				client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/files/batch-changes/123", mock.Anything).
					Run(func(args mock.Arguments) {
						req.Body = args[3].(*io.PipeReader)
					}).
					Return(req, nil).
					Once()

				resp := &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte{})),
				}
				entry := &multipartFormEntry{
					fileName: "hello.txt",
					content:  "hello world!",
				}
				requestMatcher := multipartFormRequestMatcher(entry)
				client.On("Do", mock.MatchedBy(requestMatcher)).
					Return(resp, nil).
					Once()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// TODO: use TempDir when https://github.com/golang/go/issues/51442 is cherry-picked into 1.18 or upgrade to 1.19+
			//tempDir := t.TempDir()
			workingDir, err := os.MkdirTemp("", test.name)
			require.NoError(t, err)
			t.Cleanup(func() {
				os.RemoveAll(workingDir)
			})

			if test.setup != nil {
				err := test.setup(workingDir)
				require.NoError(t, err)
			}

			client := new(mockclient.Client)
			svc := service.New(&service.Opts{Client: client})

			if test.mockInvokes != nil {
				test.mockInvokes(client)
			}

			err = svc.UploadBatchSpecWorkspaceFiles(context.Background(), workingDir, "123", test.steps)
			if test.expectedError != nil {
				assert.Equal(t, test.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			client.AssertExpectations(t)
		})
	}
}
