package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/mock"

	mockclient "github.com/sourcegraph/src-cli/internal/api/mock"
	"github.com/sourcegraph/src-cli/internal/version"
)

func TestReadConfig(t *testing.T) {
	tests := []struct {
		name         string
		fileContents *config
		envToken     string
		envFooHeader string
		envHeaders   string
		envEndpoint  string
		flagEndpoint string
		want         *config
		wantErr      string
	}{
		{
			name: "defaults",
			want: &config{
				Endpoint:          "https://sourcegraph.com",
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name: "config file, no overrides, trim slash",
			fileContents: &config{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
			},
			want: &config{
				Endpoint:          "https://example.com",
				AccessToken:       "deadbeef",
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name: "config file, token override only",
			fileContents: &config{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
			},
			envToken: "abc",
			want:     nil,
			wantErr:  errConfigMerge.Error(),
		},
		{
			name: "config file, endpoint override only",
			fileContents: &config{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
			},
			envEndpoint: "https://exmaple2.com",
			want:        nil,
			wantErr:     errConfigMerge.Error(),
		},
		{
			name: "config file, both override",
			fileContents: &config{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
			},
			envToken:    "abc",
			envEndpoint: "https://override.com",
			want: &config{
				Endpoint:          "https://override.com",
				AccessToken:       "abc",
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name:     "no config file, token from environment",
			envToken: "abc",
			want: &config{
				Endpoint:          "https://sourcegraph.com",
				AccessToken:       "abc",
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name:        "no config file, endpoint from environment",
			envEndpoint: "https://example.com",
			want: &config{
				Endpoint:          "https://example.com",
				AccessToken:       "",
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name:        "no config file, both variables",
			envEndpoint: "https://example.com",
			envToken:    "abc",
			want: &config{
				Endpoint:          "https://example.com",
				AccessToken:       "abc",
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name:         "endpoint flag should override config",
			flagEndpoint: "https://override.com/",
			fileContents: &config{
				Endpoint:          "https://example.com/",
				AccessToken:       "deadbeef",
				AdditionalHeaders: map[string]string{},
			},
			want: &config{
				Endpoint:          "https://override.com",
				AccessToken:       "deadbeef",
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name:         "endpoint flag should override environment",
			flagEndpoint: "https://override.com/",
			envEndpoint:  "https://example.com",
			envToken:     "abc",
			want: &config{
				Endpoint:          "https://override.com",
				AccessToken:       "abc",
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name:         "additional header (with SRC_HEADER_ prefix)",
			flagEndpoint: "https://override.com/",
			envEndpoint:  "https://example.com",
			envToken:     "abc",
			envFooHeader: "bar",
			want: &config{
				Endpoint:          "https://override.com",
				AccessToken:       "abc",
				AdditionalHeaders: map[string]string{"foo": "bar"},
			},
		},
		{
			name:         "additional headers (with SRC_HEADERS key)",
			flagEndpoint: "https://override.com/",
			envEndpoint:  "https://example.com",
			envToken:     "abc",
			envHeaders:   "foo:bar\nfoo-bar:bar-baz",
			want: &config{
				Endpoint:          "https://override.com",
				AccessToken:       "abc",
				AdditionalHeaders: map[string]string{"foo-bar": "bar-baz", "foo": "bar"},
			},
		},
		{
			name:        "additional headers SRC_HEADERS_AUTHORIZATION and SRC_ACCESS_TOKEN",
			envToken:    "abc",
			envEndpoint: "https://override.com",
			envHeaders:  "Authorization:Bearer",
			wantErr:     errConfigAuthorizationConflict.Error(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setEnv := func(name, val string) {
				old := os.Getenv(name)
				if err := os.Setenv(name, val); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Setenv(name, old) })
			}
			setEnv("SRC_ACCESS_TOKEN", test.envToken)
			setEnv("SRC_ENDPOINT", test.envEndpoint)

			tmpDir := t.TempDir()
			testHomeDir = tmpDir

			if test.flagEndpoint != "" {
				val := test.flagEndpoint
				endpoint = &val
				t.Cleanup(func() { endpoint = nil })
			}

			if test.fileContents != nil {
				oldConfigPath := *configPath
				t.Cleanup(func() { *configPath = oldConfigPath })

				data, err := json.Marshal(*test.fileContents)
				if err != nil {
					t.Fatal(err)
				}
				filePath := filepath.Join(tmpDir, "config.json")
				err = os.WriteFile(filePath, data, 0600)
				if err != nil {
					t.Fatal(err)
				}
				*configPath = filePath
			}

			if err := os.Setenv("SRC_HEADER_FOO", test.envFooHeader); err != nil {
				t.Fatal(err)
			}

			if err := os.Setenv("SRC_HEADERS", test.envHeaders); err != nil {
				t.Fatal(err)
			}

			config, err := readConfig()
			if diff := cmp.Diff(test.want, config); diff != "" {
				t.Errorf("config: %v", diff)
			}
			var errMsg string
			if err != nil {
				errMsg = err.Error()
			}
			if diff := cmp.Diff(test.wantErr, errMsg); diff != "" {
				t.Errorf("err: %v", diff)
			}
		})
	}
}

func TestCheckRecommendedVersion(t *testing.T) {
	tests := []struct {
		name               string
		buildTag           string
		recommendedVersion string
		expectedWarning    string
	}{
		{
			name:               "Unknown version",
			buildTag:           "4.3.0",
			recommendedVersion: "",
			expectedWarning:    "Recommended version: <unknown>\nThis Sourcegraph instance does not support this feature.\n",
		},
		{
			name:               "Same versions",
			buildTag:           "5.2.1",
			recommendedVersion: "5.2.1",
			expectedWarning:    "",
		},
		{
			name:               "Outdated version",
			buildTag:           "4.3.0",
			recommendedVersion: "5.2.1",
			expectedWarning:    "⚠️  You are using an outdated version 4.3.0. Please upgrade to 5.2.1 or later.\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version.BuildTag = test.buildTag

			client := &mockclient.Client{}

			req := httptest.NewRequest(http.MethodGet, "http://fake.com/.api/src-cli/version", nil)
			client.On("NewHTTPRequest", mock.Anything, http.MethodGet, ".api/src-cli/version", mock.Anything).
				Return(req, nil).
				Once()

			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(fmt.Sprintf(`{"version": "%s"}`, test.recommendedVersion))),
			}

			client.On("Do", mock.Anything).
				Return(resp, nil).
				Once()

			output := redirectStdout(t)

			checkForOutdatedVersion(client)

			actualOutput := output.String()

			client.AssertExpectations(t)

			if actualOutput != test.expectedWarning {
				t.Errorf("Expected warning message: %s, got: %s", test.expectedWarning, actualOutput)
			}
		})
	}
}

// Redirect stdout for testing
func redirectStdout(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	log.SetFlags(0)
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(os.Stdout)
	})
	return &buf
}
