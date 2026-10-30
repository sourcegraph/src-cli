package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/oauth"
)

const SGDotComEndpoint = "https://sourcegraph.com"

var (
	verbose = flag.Bool("v", false, "print verbose output")

	// The following arguments are deprecated which is why they are no longer documented
	configPath   = flag.String("config", "", "")
	endpointFlag = flag.String("endpoint", "", "")

	errConfigMerge                 = errors.New("when using a configuration file, zero or all environment variables must be set")
	errConfigAuthorizationConflict = errors.New("when passing an 'Authorization' additional headers, SRC_ACCESS_TOKEN must never be set")
	errCIAccessTokenRequired       = errors.New("CI is true and SRC_ACCESS_TOKEN is not set or empty. When running in CI OAuth tokens cannot be used, only SRC_ACCESS_TOKEN. Either set CI=false or define a SRC_ACCESS_TOKEN")
)

// commands contains all registered subcommands.
var commands commander

func main() {
	// Configure logging.
	log.SetFlags(0)
	log.SetPrefix("")

	ranMigratedCmd, exitCode, err := maybeRunMigratedCommand()
	if ranMigratedCmd {
		if err != nil {
			log.Println(err)
		}
		os.Exit(exitCode)
	}

	// if we didn't run a migrated command, then lets try running the legacy version
	commands.run(flag.CommandLine, "src", usageText(), normalizeDashHelp(os.Args[1:]))
}

// normalizeDashHelp converts --help to -help since Go's flag parser only supports single dash.
func normalizeDashHelp(args []string) []string {
	args = slices.Clone(args)
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--help" {
			args[i] = "-help"
		}
	}
	return args
}

func parseEndpoint(endpoint string) (*url.URL, error) {
	u, err := url.ParseRequestURI(strings.TrimSuffix(endpoint, "/"))
	if err != nil {
		return nil, err
	}
	if !(u.Scheme == "http" || u.Scheme == "https") {
		return nil, errors.Newf("invalid scheme %s: require http or https", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.Newf("empty host")
	}
	// auth in the URL is not used, and could be explosed in log output.
	// Explicitly clear it in case it's accidentally set in SRC_ENDPOINT or the config file.
	u.User = nil
	return u, nil
}

var cfg *config

// config holds the resolved configuration used at runtime.
type config struct {
	accessToken          string
	additionalHeaders    map[string]string
	proxyURL             *url.URL
	proxyPath            string
	configFilePath       string
	endpointURL          *url.URL // always non-nil; defaults to https://sourcegraph.com via readConfig
	usingDefaultEndpoint bool
	inCI                 bool
}

// configFromFile holds the config as read from the config file,
// which is validated and parsed into the config struct.
type configFromFile struct {
	Endpoint          string            `json:"endpoint"`
	AccessToken       string            `json:"accessToken"`
	AdditionalHeaders map[string]string `json:"additionalHeaders"`
	Proxy             string            `json:"proxy"`
}

type AuthMode int

const (
	AuthModeOAuth AuthMode = iota
	AuthModeAccessToken
)

func (c *config) AuthMode() AuthMode {
	if c.accessToken != "" {
		return AuthModeAccessToken
	}
	return AuthModeOAuth
}

func (c *config) InCI() bool {
	return c.inCI
}

func (c *config) requireCIAccessToken() error {
	// In CI we typically do not have access to the keyring and the machine is also typically headless
	// we therefore require SRC_ACCESS_TOKEN to be set when in CI.
	// If someone really wants to run with OAuth in CI they can temporarily do CI=false
	if c.InCI() && c.AuthMode() != AuthModeAccessToken {
		return errCIAccessTokenRequired
	}

	return nil
}

// apiClient returns an api.Client built from the configuration.
func (c *config) apiClient(flags *api.Flags, out io.Writer) api.Client {
	opts := api.ClientOpts{
		EndpointURL:            c.endpointURL,
		AccessToken:            c.accessToken,
		AdditionalHeaders:      c.additionalHeaders,
		Flags:                  flags,
		Out:                    out,
		ProxyURL:               c.proxyURL,
		ProxyPath:              c.proxyPath,
		RequireAccessTokenInCI: c.InCI(),
	}

	// Only use OAuth if we do not have SRC_ACCESS_TOKEN set
	if c.accessToken == "" {
		if t, err := oauth.LoadToken(context.Background(), c.endpointURL); err == nil {
			opts.OAuthToken = t
		}
	}

	return api.NewClient(opts)
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

	var cfgFromFile configFromFile
	var cfg config
	cfg.inCI = isCI()
	var endpointStr string
	var proxyStr string
	if err == nil {
		cfg.configFilePath = cfgPath
		if err := json.Unmarshal(data, &cfgFromFile); err != nil {
			return nil, err
		}
		endpointStr = cfgFromFile.Endpoint
		cfg.accessToken = cfgFromFile.AccessToken
		cfg.additionalHeaders = cfgFromFile.AdditionalHeaders
		proxyStr = cfgFromFile.Proxy
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
		cfg.accessToken = envToken
	}
	if envEndpoint != "" {
		endpointStr = envEndpoint
	}
	if endpointStr == "" {
		endpointStr = SGDotComEndpoint
		cfg.usingDefaultEndpoint = true
	}
	if envProxy != "" {
		proxyStr = envProxy
	}

	// Lastly, apply endpoint flag if set
	if endpointFlag != nil && *endpointFlag != "" {
		endpointStr = *endpointFlag
	}

	if endpointURL, err := parseEndpoint(endpointStr); err != nil {
		return nil, errors.Newf("invalid endpoint: %s", endpointStr)
	} else {
		cfg.endpointURL = endpointURL
	}

	if proxyStr != "" {

		parseProxyEndpoint := func(endpoint string) (scheme string, address string) {
			parts := strings.SplitN(endpoint, "://", 2)
			if len(parts) == 2 {
				return parts[0], parts[1]
			}
			return "", endpoint
		}

		urlSchemes := []string{"http", "https", "socks", "socks5", "socks5h"}

		isURLScheme := func(scheme string) bool {
			return slices.Contains(urlSchemes, scheme)
		}

		scheme, address := parseProxyEndpoint(proxyStr)

		if isURLScheme(scheme) {
			endpoint := proxyStr
			// assume socks means socks5, because that's all we support
			if scheme == "socks" {
				endpoint = "socks5://" + address
			}
			cfg.proxyURL, err = url.Parse(endpoint)
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
				return nil, errors.Newf("invalid proxy configuration: %w", err)
			}
			if !isValidUDS {
				return nil, errors.Newf("invalid proxy socket: %s", path)
			}
			cfg.proxyPath = path
		} else {
			return nil, errors.Newf("invalid proxy endpoint: %s", proxyStr)
		}
	}

	// Merge config-file headers under the env headers (which take precedence),
	// lowercasing config keys to match the env-header convention.
	envHeaders := parseAdditionalHeaders()
	for k, v := range cfg.additionalHeaders {
		lk := strings.ToLower(k)
		if _, ok := envHeaders[lk]; !ok {
			envHeaders[lk] = v
		}
	}
	cfg.additionalHeaders = envHeaders
	// Ensure that we're not clashing additonal headers
	_, hasAuthorizationAdditonalHeader := cfg.additionalHeaders["authorization"]
	if cfg.accessToken != "" && hasAuthorizationAdditonalHeader {
		return nil, errConfigAuthorizationConflict
	}

	return &cfg, nil
}

func isCI() bool {
	value, ok := os.LookupEnv("CI")
	return ok && value != ""
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
		return false, errors.Newf("not a UNIX Domain Socket: %v: %w", path, err)
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
