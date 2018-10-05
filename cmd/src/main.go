package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const (
	AccessTokenEnvVar = "SRC_ACCESS_TOKEN"
	ConfigEnvVar      = "SRC_CONFIG"
	ConfigFilename    = "src-config.json"
	DefaultEndpoint   = "https://sourcegraph.com"
	EndpointEnvVar    = "SRC_ENDPOINT"
)

const usageText = `src is a tool that provides access to Sourcegraph instances.
For more information, see https://github.com/sourcegraph/src-cli

Usage:

	src [options] command [command options]

The options are:

	-config=$HOME/src-config.json    specifies a file containing {"accessToken": "<secret>", "endpoint": "https://sourcegraph.com"}
      You can use "${}" syntax to reference environment variables, even on Windows,
      but you may wish to use single-quotes to protect that syntax from shell expansion:

      -config=${UserProfile}/src-config.json or
      -config='${TMPDIR}/my-config.json'

      [Environment Variables]
      $SRC_CONFIG       can point to the config file
                        although the -config option is always authoritative
      $SRC_ACCESS_TOKEN can specify, or supersede, the access token

	-endpoint=                       specifies the endpoint to use e.g. "https://sourcegraph.com" (overrides -config, if any)
      [Environment Variables]
      $SRC_ENDPOINT     can specify, or supersede, the value in -config
                        although the -endpoint option is always authoritative

The commands are:

	search          search for results on Sourcegraph
	api             interacts with the Sourcegraph GraphQL API
	repos,repo      manages repositories
	users,user      manages users
	orgs,org        manages organizations
	config          manages global, org, and user settings
	extensions,ext  manages extensions (experimental)

Use "src [command] -h" for more information about a command.

`

var (
	configPath = flag.String("config", "", "")
	endpoint   = flag.String("endpoint", "", "")
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
	Endpoint    string `json:"endpoint"`
	AccessToken string `json:"accessToken"`
}

// readConfig reads the config file from the given path, plus any in-scope environment variables
func readConfig() (*config, error) {
	cfgPath := *configPath
	userSpecified := cfgPath != ""
	if !userSpecified {
		if cfgEnvPath := os.Getenv(ConfigEnvVar); cfgEnvPath != "" {
			cfgPath = cfgEnvPath
			userSpecified = true
		}
	}
	if !userSpecified {
		currentUser, err := user.Current()
		if err != nil {
			return nil, err
		}
		cfgPath = filepath.Join(currentUser.HomeDir, ConfigFilename)
	}
	data, err := ioutil.ReadFile(os.ExpandEnv(cfgPath))
	if err != nil && (!os.IsNotExist(err) || userSpecified) {
		return nil, err
	}
	var cfg config
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	}

	// Apply config overrides.
	if envToken := os.Getenv(AccessTokenEnvVar); envToken != "" {
		cfg.AccessToken = envToken
	}
	userEndpoint := *endpoint
	if envEndpoint := os.Getenv(EndpointEnvVar); userEndpoint == "" && envEndpoint != "" {
		userEndpoint = envEndpoint
	}
	if userEndpoint != "" {
		cfg.Endpoint = strings.TrimSuffix(userEndpoint, "/")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}
	return &cfg, nil
}
