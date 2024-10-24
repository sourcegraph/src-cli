package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/sourcegraph/src-cli/internal/api"
)

func TestReadConfig(t *testing.T) {
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
		fileContents *config
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
				Endpoint:          "https://sourcegraph.com",
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name: "config file, no overrides, trim slash",
			fileContents: &config{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
				Proxy:       "https://proxy.com:8080",
			},
			want: &config{
				Endpoint:          "https://example.com",
				AccessToken:       "deadbeef",
				AdditionalHeaders: map[string]string{},
				Proxy:             "https://proxy.com:8080",
				ProxyPath:         "",
				ProxyURL: &url.URL{
					Scheme: "https",
					Host:   "proxy.com:8080",
				},
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
			name: "config file, proxy override only (allow)",
			fileContents: &config{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
				Proxy:       "https://proxy.com:8080",
			},
			envProxy: "socks5://other.proxy.com:9999",
			want: &config{
				Endpoint:    "https://example.com",
				AccessToken: "deadbeef",
				Proxy:       "socks5://other.proxy.com:9999",
				ProxyPath:   "",
				ProxyURL: &url.URL{
					Scheme: "socks5",
					Host:   "other.proxy.com:9999",
				},
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name: "config file, all override",
			fileContents: &config{
				Endpoint:    "https://example.com/",
				AccessToken: "deadbeef",
				Proxy:       "https://proxy.com:8080",
			},
			envToken:    "abc",
			envEndpoint: "https://override.com",
			envProxy:    "socks5://other.proxy.com:9999",
			want: &config{
				Endpoint:    "https://override.com",
				AccessToken: "abc",
				Proxy:       "socks5://other.proxy.com:9999",
				ProxyPath:   "",
				ProxyURL: &url.URL{
					Scheme: "socks5",
					Host:   "other.proxy.com:9999",
				},
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
			name:     "no config file, proxy from environment",
			envProxy: "https://proxy.com:8080",
			want: &config{
				Endpoint:    "https://sourcegraph.com",
				AccessToken: "",
				Proxy:       "https://proxy.com:8080",
				ProxyPath:   "",
				ProxyURL: &url.URL{
					Scheme: "https",
					Host:   "proxy.com:8080",
				},
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name:        "no config file, all variables",
			envEndpoint: "https://example.com",
			envToken:    "abc",
			envProxy:    "https://proxy.com:8080",
			want: &config{
				Endpoint:    "https://example.com",
				AccessToken: "abc",
				Proxy:       "https://proxy.com:8080",
				ProxyPath:   "",
				ProxyURL: &url.URL{
					Scheme: "https",
					Host:   "proxy.com:8080",
				},
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name:     "UNIX Domain Socket proxy using scheme and absolute path",
			envProxy: "unix://" + socketPath,
			want: &config{
				Endpoint:          "https://sourcegraph.com",
				Proxy:             "unix://" + socketPath,
				ProxyPath:         socketPath,
				ProxyURL:          nil,
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name:     "UNIX Domain Socket proxy with absolute path",
			envProxy: socketPath,
			want: &config{
				Endpoint:          "https://sourcegraph.com",
				Proxy:             socketPath,
				ProxyPath:         socketPath,
				ProxyURL:          nil,
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name:     "socks --> socks5",
			envProxy: "socks://localhost:1080",
			want: &config{
				Endpoint:  "https://sourcegraph.com",
				Proxy:     "socks://localhost:1080",
				ProxyPath: "",
				ProxyURL: &url.URL{
					Scheme: "socks5",
					Host:   "localhost:1080",
				},
				AdditionalHeaders: map[string]string{},
			},
		},
		{
			name:     "socks5h",
			envProxy: "socks5h://localhost:1080",
			want: &config{
				Endpoint:  "https://sourcegraph.com",
				Proxy:     "socks5h://localhost:1080",
				ProxyPath: "",
				ProxyURL: &url.URL{
					Scheme: "socks5h",
					Host:   "localhost:1080",
				},
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
			setEnv("SRC_PROXY", test.envProxy)

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
