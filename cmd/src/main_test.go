package main

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/api"
)

func TestReadConfig(t *testing.T) {
	// UNIX Domain Sockets have a max path length: 104 on BSD/macOS, 108 on Linux.
	// Including a prefix and suffix was causing the path to be too long
	// with t.TempDir() (os.TempDir() is a shorter path) so we don't use them.
	socketPath, err := api.CreateTempFile(t.TempDir(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	socketServer, err := api.StartUnixSocketServer(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer socketServer.Stop()
	defer os.Remove(socketPath)

	tests := []struct {
		name         string
		fileContents *configFromFile
		envCI        string
		envToken     string
		envFooHeader string
		envHeaders   string
		envEndpoint  string
		envProxy     string
		flagEndpoint string
		want         *config
		wantErr      string
	}{
		{
			name: "defaults",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "sourcegraph.com",
				},
				additionalHeaders: map[string]string{},
			},
		},
		{
			name: "config file, no overrides, trim slash",
			fileContents: &configFromFile{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
				Proxy:       "https://proxy.com:8080",
			},
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "example.com",
				},
				accessToken:       "deadbeef",
				additionalHeaders: map[string]string{},
				proxyPath:         "",
				proxyURL: &url.URL{
					Scheme: "https",
					Host:   "proxy.com:8080",
				},
			},
		},
		{
			name: "config file, token override only",
			fileContents: &configFromFile{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
			},
			envToken: "abc",
			want:     nil,
			wantErr:  errConfigMerge.Error(),
		},
		{
			name: "config file, endpoint override only",
			fileContents: &configFromFile{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
			},
			envEndpoint: "https://exmaple2.com",
			want:        nil,
			wantErr:     errConfigMerge.Error(),
		},
		{
			name: "config file, proxy override only (allow)",
			fileContents: &configFromFile{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
				Proxy:       "https://proxy.com:8080",
			},
			envProxy: "socks5://other.proxy.com:9999",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "example.com",
				},
				accessToken: "deadbeef",
				proxyPath:   "",
				proxyURL: &url.URL{
					Scheme: "socks5",
					Host:   "other.proxy.com:9999",
				},
				additionalHeaders: map[string]string{},
			},
		},
		{
			name: "config file, all override",
			fileContents: &configFromFile{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
				Proxy:       "https://proxy.com:8080",
			},
			envToken:    "abc",
			envEndpoint: "https://override.com",
			envProxy:    "socks5://other.proxy.com:9999",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "override.com",
				},
				accessToken: "abc",
				proxyPath:   "",
				proxyURL: &url.URL{
					Scheme: "socks5",
					Host:   "other.proxy.com:9999",
				},
				additionalHeaders: map[string]string{},
			},
		},
		{
			name:     "no config file, token from environment",
			envToken: "abc",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "sourcegraph.com",
				},
				accessToken:       "abc",
				additionalHeaders: map[string]string{},
			},
		},
		{
			name:        "no config file, endpoint from environment",
			envEndpoint: "https://example.com",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "example.com",
				},
				accessToken:       "",
				additionalHeaders: map[string]string{},
			},
		},
		{
			name:     "no config file, proxy from environment",
			envProxy: "https://proxy.com:8080",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "sourcegraph.com",
				},
				accessToken: "",
				proxyPath:   "",
				proxyURL: &url.URL{
					Scheme: "https",
					Host:   "proxy.com:8080",
				},
				additionalHeaders: map[string]string{},
			},
		},
		{
			name:        "no config file, all variables",
			envEndpoint: "https://example.com",
			envToken:    "abc",
			envProxy:    "https://proxy.com:8080",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "example.com",
				},
				accessToken: "abc",
				proxyPath:   "",
				proxyURL: &url.URL{
					Scheme: "https",
					Host:   "proxy.com:8080",
				},
				additionalHeaders: map[string]string{},
			},
		},
		{
			name:     "UNIX Domain Socket proxy using scheme and absolute path",
			envProxy: "unix://" + socketPath,
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "sourcegraph.com",
				},
				proxyPath:         socketPath,
				proxyURL:          nil,
				additionalHeaders: map[string]string{},
			},
		},
		{
			name:     "UNIX Domain Socket proxy with absolute path",
			envProxy: socketPath,
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "sourcegraph.com",
				},
				proxyPath:         socketPath,
				proxyURL:          nil,
				additionalHeaders: map[string]string{},
			},
		},
		{
			name:     "socks --> socks5",
			envProxy: "socks://localhost:1080",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "sourcegraph.com",
				},
				proxyPath: "",
				proxyURL: &url.URL{
					Scheme: "socks5",
					Host:   "localhost:1080",
				},
				additionalHeaders: map[string]string{},
			},
		},
		{
			name:     "socks5h",
			envProxy: "socks5h://localhost:1080",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "sourcegraph.com",
				},
				proxyPath: "",
				proxyURL: &url.URL{
					Scheme: "socks5h",
					Host:   "localhost:1080",
				},
				additionalHeaders: map[string]string{},
			},
		},
		{
			name:         "endpoint flag should override config",
			flagEndpoint: "https://override.com/",
			fileContents: &configFromFile{
				Endpoint:          "https://example.com/",
				AccessToken:       "deadbeef",
				AdditionalHeaders: map[string]string{},
			},
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "override.com",
				},
				accessToken:       "deadbeef",
				additionalHeaders: map[string]string{},
			},
		},
		{
			name:         "endpoint flag should override environment",
			flagEndpoint: "https://override.com/",
			envEndpoint:  "https://example.com",
			envToken:     "abc",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "override.com",
				},
				accessToken:       "abc",
				additionalHeaders: map[string]string{},
			},
		},
		{
			name:         "additional header (with SRC_HEADER_ prefix)",
			flagEndpoint: "https://override.com/",
			envEndpoint:  "https://example.com",
			envToken:     "abc",
			envFooHeader: "bar",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "override.com",
				},
				accessToken:       "abc",
				additionalHeaders: map[string]string{"foo": "bar"},
			},
		},
		{
			name:         "additional headers (with SRC_HEADERS key)",
			flagEndpoint: "https://override.com/",
			envEndpoint:  "https://example.com",
			envToken:     "abc",
			envHeaders:   "foo:bar\nfoo-bar:bar-baz",
			want: &config{
				endpointURL: &url.URL{
					Scheme: "https",
					Host:   "override.com",
				},
				accessToken:       "abc",
				additionalHeaders: map[string]string{"foo-bar": "bar-baz", "foo": "bar"},
			},
		},
		{
			name:        "additional headers SRC_HEADERS_AUTHORIZATION and SRC_ACCESS_TOKEN",
			envToken:    "abc",
			envEndpoint: "https://override.com",
			envHeaders:  "Authorization:Bearer",
			wantErr:     errConfigAuthorizationConflict.Error(),
		},
		{
			name:  "CI does not require access token during config read",
			envCI: "1",
			want: &config{
				endpointURL:       &url.URL{Scheme: "https", Host: "sourcegraph.com"},
				additionalHeaders: map[string]string{},
				inCI:              true,
			},
		},
		{
			name:  "CI allows access token from config file",
			envCI: "1",
			fileContents: &configFromFile{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
			},
			want: &config{
				endpointURL:       &url.URL{Scheme: "https", Host: "example.com"},
				accessToken:       "deadbeef",
				additionalHeaders: map[string]string{},
				inCI:              true,
			},
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
			setEnv("SRC_PROXY", test.envProxy)
			setEnv("CI", test.envCI)

			tmpDir := t.TempDir()
			testHomeDir = tmpDir

			if test.flagEndpoint != "" {
				val := test.flagEndpoint
				endpointFlag = &val
				t.Cleanup(func() { endpointFlag = nil })
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

			got, err := readConfig()
			if diff := cmp.Diff(test.want, got,
				cmp.AllowUnexported(config{}),
				cmpopts.IgnoreFields(config{}, "configFilePath"),
			); diff != "" {
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

func TestConfigAuthMode(t *testing.T) {
	t.Run("oauth when access token is empty", func(t *testing.T) {
		if got := (&config{}).AuthMode(); got != AuthModeOAuth {
			t.Fatalf("AuthMode() = %v, want %v", got, AuthModeOAuth)
		}
	})

	t.Run("access token when configured", func(t *testing.T) {
		if got := (&config{accessToken: "token"}).AuthMode(); got != AuthModeAccessToken {
			t.Fatalf("AuthMode() = %v, want %v", got, AuthModeAccessToken)
		}
	})
}

func TestConfigAPIClientCIAccessTokenGate(t *testing.T) {
	endpointURL := &url.URL{Scheme: "https", Host: "example.com"}

	t.Run("requires access token in CI", func(t *testing.T) {
		client := (&config{endpointURL: endpointURL, inCI: true}).apiClient(nil, io.Discard)

		_, err := client.NewHTTPRequest(context.Background(), "GET", ".api/src-cli/version", nil)
		if !errors.Is(err, api.ErrCIAccessTokenRequired) {
			t.Fatalf("NewHTTPRequest() error = %v, want %v", err, api.ErrCIAccessTokenRequired)
		}
	})

	t.Run("allows access token in CI", func(t *testing.T) {
		client := (&config{endpointURL: endpointURL, inCI: true, accessToken: "abc"}).apiClient(nil, io.Discard)

		req, err := client.NewHTTPRequest(context.Background(), "GET", ".api/src-cli/version", nil)
		if err != nil {
			t.Fatalf("NewHTTPRequest() unexpected error: %s", err)
		}
		if got := req.Header.Get("Authorization"); got != "token abc" {
			t.Fatalf("Authorization header = %q, want %q", got, "token abc")
		}
	})

	t.Run("allows oauth mode outside CI", func(t *testing.T) {
		client := (&config{endpointURL: endpointURL}).apiClient(nil, io.Discard)

		if _, err := client.NewHTTPRequest(context.Background(), "GET", ".api/src-cli/version", nil); err != nil {
			t.Fatalf("NewHTTPRequest() unexpected error: %s", err)
		}
	})
}
