package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
)

const usageText = `src is a tool that provides access to Sourcegraph instances.
For more information, see https://github.com/sourcegraph/src-cli

Usage:

	src [options] command [command options]

Environment variables
	SRC_ACCESS_TOKEN  Sourcegraph access token
	SRC_ENDPOINT      endpoint to use, if unset will default to "https://sourcegraph.com"
	SRC_PROXY         A proxy to use for proxying requests to the Sourcegraph endpoint.
	                  Supports HTTP(S), SOCKS5/5h, and UNIX Domain Socket proxies.
					  If a UNIX Domain Socket, the path can be either an absolute path,
					  or can start with ~/ or %USERPROFILE%\ for a path in the user's home directory.
					  Examples:
						- https://localhost:3080
						- https://<user>:<password>localhost:8080
						- socks5h://localhost:1080
						- socks5://<username>:<password>@localhost:1080
						- unix://~/src-proxy.sock
						- unix://%USERPROFILE%\src-proxy.sock
						- ~/src-proxy.sock
						- %USERPROFILE%\src-proxy.sock
						- C:\some\path\src-proxy.sock

The options are:

	-v                               print verbose output

The commands are:

	api             interacts with the Sourcegraph GraphQL API
	batch           manages batch changes
	code-intel      manages code intelligence data
	config          manages global, org, and user settings
	extensions,ext  manages extensions (experimental)
	extsvc          manages external services
	gateway         interacts with Cody Gateway
	login           authenticate to a Sourcegraph instance with your user credentials
	orgs,org        manages organizations
	teams,team      manages teams
	repos,repo      manages repositories
	sbom            manages SBOM (Software Bill of Materials) data
	search          search for results on Sourcegraph
	serve-git       serves your local git repositories over HTTP for Sourcegraph to pull
	users,user      manages users
	codeowners      manages code ownership information
	version         display and compare the src-cli version against the recommended version for your instance

Use "src [command] -h" for more information about a command.

`

var (
	verbose = flag.Bool("v", false, "print verbose output")

	// The following arguments are deprecated which is why they are no longer documented
	configPath = flag.String("config", "", "")
	endpoint   = flag.String("endpoint", "", "")

	errConfigMerge                 = errors.New("when using a configuration file, zero or all environment variables must be set")
	errConfigAuthorizationConflict = errors.New("when passing an 'Authorization' additional headers, SRC_ACCESS_TOKEN must never be set")
)

// commands contains all registered subcommands.
var commands commander

func main() {
	// Configure logging.
	log.SetFlags(0)
	log.SetPrefix("")

	commands.run(flag.CommandLine, "src", usageText, os.Args[1:])
}

var cfg *config

// config represents the config format.
type config struct {
	Endpoint          string            `json:"endpoint"`
	AccessToken       string            `json:"accessToken"`
	AdditionalHeaders map[string]string `json:"additionalHeaders"`
	Proxy             string            `json:"proxy"`
	ProxyURL          *url.URL
	ProxyPath         string
	ConfigFilePath    string
}

// apiClient returns an api.Client built from the configuration.
func (c *config) apiClient(flags *api.Flags, out io.Writer) api.Client {
	return api.NewClient(api.ClientOpts{
		Endpoint:          c.Endpoint,
		AccessToken:       c.AccessToken,
		AdditionalHeaders: c.AdditionalHeaders,
		Flags:             flags,
		Out:               out,
		ProxyURL:          c.ProxyURL,
		ProxyPath:         c.ProxyPath,
	})
}

// readConfig reads the config file from the given path.
func readConfig() (*config, error) {
	cfgFile := *configPath
	userSpecified := *configPath != ""

	if !userSpecified {
		cfgFile = "~/src-config.json"
	}

	cfgPath, err := expandHomeDir(cfgFile)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(os.ExpandEnv(cfgPath))
	if err != nil && (!os.IsNotExist(err) || userSpecified) {
		return nil, err
	}
	var cfg config
	if err == nil {
		cfg.ConfigFilePath = cfgPath
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	}

	envToken := os.Getenv("SRC_ACCESS_TOKEN")
	envEndpoint := os.Getenv("SRC_ENDPOINT")
	envProxy := os.Getenv("SRC_PROXY")

	if userSpecified {
		// If a config file is present, either zero or both required environment variables must be present.
		// We don't want to partially apply environment variables.
		// Note that SRC_PROXY is optional so we don't test for it.
		if envToken == "" && envEndpoint != "" {
			return nil, errConfigMerge
		}
		if envToken != "" && envEndpoint == "" {
			return nil, errConfigMerge
		}
	}

	// Apply config overrides.
	if envToken != "" {
		cfg.AccessToken = envToken
	}
	if envEndpoint != "" {
		cfg.Endpoint = envEndpoint
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://sourcegraph.com"
	}
	if envProxy != "" {
		cfg.Proxy = envProxy
	}

	if cfg.Proxy != "" {

		parseEndpoint := func(endpoint string) (scheme string, address string) {
			parts := strings.SplitN(endpoint, "://", 2)
			if len(parts) == 2 {
				return parts[0], parts[1]
			}
			return "", endpoint
		}

		urlSchemes := []string{"http", "https", "socks", "socks5", "socks5h"}

		isURLScheme := func(scheme string) bool {
			for _, s := range urlSchemes {
				if scheme == s {
					return true
				}
			}
			return false
		}

		scheme, address := parseEndpoint(cfg.Proxy)

		if isURLScheme(scheme) {
			endpoint := cfg.Proxy
			// assume socks means socks5, because that's all we support
			if scheme == "socks" {
				endpoint = "socks5://" + address
			}
			cfg.ProxyURL, err = url.Parse(endpoint)
			if err != nil {
				return nil, err
			}
		} else if scheme == "" || scheme == "unix" {
			path, err := expandHomeDir(address)
			if err != nil {
				return nil, err
			}
			isValidUDS, err := isValidUnixSocket(path)
			if err != nil {
				return nil, errors.Newf("Invalid proxy configuration: %w", err)
			}
			if !isValidUDS {
				return nil, errors.Newf("invalid proxy socket: %s", path)
			}
			cfg.ProxyPath = path
		} else {
			return nil, errors.Newf("invalid proxy endpoint: %s", cfg.Proxy)
		}
	}

	cfg.AdditionalHeaders = parseAdditionalHeaders()
	// Ensure that we're not clashing additonal headers
	_, hasAuthorizationAdditonalHeader := cfg.AdditionalHeaders["authorization"]
	if cfg.AccessToken != "" && hasAuthorizationAdditonalHeader {
		return nil, errConfigAuthorizationConflict
	}

	// Lastly, apply endpoint flag if set
	if endpoint != nil && *endpoint != "" {
		cfg.Endpoint = *endpoint
	}

	cfg.Endpoint = cleanEndpoint(cfg.Endpoint)

	return &cfg, nil
}

func cleanEndpoint(urlStr string) string {
	return strings.TrimSuffix(urlStr, "/")
}

// isValidUnixSocket checks if the given path is a valid Unix socket.
//
// Parameters:
//   - path: A string representing the file path to check.
//
// Returns:
//   - bool: true if the path is a valid Unix socket, false otherwise.
//   - error: nil if the check was successful, or an error if an unexpected issue occurred.
//
// The function attempts to establish a connection to the Unix socket at the given path.
// If the connection succeeds, it's considered a valid Unix socket.
// If the file doesn't exist, it returns false without an error.
// For any other errors, it returns false and the encountered error.
func isValidUnixSocket(path string) (bool, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Newf("Not a UNIX Domain Socket: %v: %w", path, err)
	}
	defer conn.Close()

	return true, nil
}

var testHomeDir string // used by tests to mock the user's $HOME

// expandHomeDir expands to the user's home directory a tilde (~) or %USERPROFILE% at the beginning of a file path.
//
// Parameters:
//   - filePath: A string representing the file path that may start with "~/" or "%USERPROFILE%\".
//
// Returns:
//   - string: The expanded file path with the home directory resolved.
//   - error: An error if the user's home directory cannot be determined.
//
// The function handles both Unix-style paths starting with "~/" and Windows-style paths starting with "%USERPROFILE%\".
// It uses the testHomeDir variable for testing purposes if set, otherwise it uses os.UserHomeDir() to get the user's home directory.
// If the input path doesn't start with either prefix, it returns the original path unchanged.
func expandHomeDir(filePath string) (string, error) {
	if strings.HasPrefix(filePath, "~/") || strings.HasPrefix(filePath, "%USERPROFILE%\\") {
		var homeDir string
		if testHomeDir != "" {
			homeDir = testHomeDir
		} else {
			hd, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			homeDir = hd
		}

		if strings.HasPrefix(filePath, "~/") {
			return filepath.Join(homeDir, filePath[2:]), nil
		}
		return filepath.Join(homeDir, filePath[14:]), nil
	}

	return filePath, nil
}
