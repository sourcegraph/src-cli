package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/oauth"
)

func init() {
	usage := `'src login' helps you authenticate 'src' to access a Sourcegraph instance with your user credentials.

Usage:

    src login [flags] SOURCEGRAPH_URL

Examples:

  Authenticate to a Sourcegraph instance at https://sourcegraph.example.com:

    $ src login https://sourcegraph.example.com

  Authenticate to Sourcegraph.com:

    $ src login https://sourcegraph.com

  Use OAuth device flow to authenticate:

    $ src login --oauth https://sourcegraph.com


  Override the default client id used during device flow when authenticating:

    $ src login --oauth https://sourcegraph.com
`

	flagSet := flag.NewFlagSet("login", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintln(flag.CommandLine.Output(), usage)
		flagSet.PrintDefaults()
	}

	var (
		apiFlags = api.NewFlags(flagSet)
		useOAuth = flagSet.Bool("oauth", false, "Use OAuth device flow to obtain an access token interactively")
	)

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}
		endpoint := cfg.Endpoint
		if flagSet.NArg() >= 1 {
			endpoint = flagSet.Arg(0)
		}
		if endpoint == "" {
			return cmderrors.Usage("expected exactly one argument: the Sourcegraph URL, or SRC_ENDPOINT to be set")
		}

		client := cfg.apiClient(apiFlags, io.Discard)

		return loginCmd(context.Background(), loginParams{
			cfg:         cfg,
			client:      client,
			endpoint:    endpoint,
			out:         os.Stdout,
			useOAuth:    *useOAuth,
			apiFlags:    apiFlags,
			oauthClient: oauth.NewClient(oauth.DefaultClientID),
		})
	}

	commands = append(commands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

type loginParams struct {
	cfg         *config
	client      api.Client
	endpoint    string
	out         io.Writer
	useOAuth    bool
	apiFlags    *api.Flags
	oauthClient oauth.Client
}

type loginFlow func(context.Context, loginParams) error

type loginFlowKind int

const (
	loginFlowOAuth loginFlowKind = iota
	loginFlowMissingAuth
	loginFlowEndpointConflict
	loginFlowValidate
)

var loadStoredOAuthToken = oauth.LoadToken

func loginCmd(ctx context.Context, p loginParams) error {
	if p.cfg.ConfigFilePath != "" {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "⚠️  Warning: Configuring src with a JSON file is deprecated. Please migrate to using the env vars SRC_ENDPOINT, SRC_ACCESS_TOKEN, and SRC_PROXY instead, and then remove %s. See https://github.com/sourcegraph/src-cli#readme for more information.\n", p.cfg.ConfigFilePath)
	}

	_, flow := selectLoginFlow(ctx, p)
	return flow(ctx, p)
}

// selectLoginFlow decides what login flow to run based on flags and config.
func selectLoginFlow(ctx context.Context, p loginParams) (loginFlowKind, loginFlow) {
	endpointArg := cleanEndpoint(p.endpoint)

	if p.useOAuth {
		return loginFlowOAuth, runOAuthLogin
	}
	if !hasEffectiveAuth(ctx, p.cfg, endpointArg) {
		return loginFlowMissingAuth, runMissingAuthLogin
	}
	if endpointArg != p.cfg.Endpoint {
		return loginFlowEndpointConflict, runEndpointConflictLogin
	}
	return loginFlowValidate, runValidatedLogin
}

// hasEffectiveAuth determines whether we have auth credentials to continue. It first checks for a resolved Access Token in
// config, then it checks for a stored OAuth token.
func hasEffectiveAuth(ctx context.Context, cfg *config, resolvedEndpoint string) bool {
	if cfg.AccessToken != "" {
		return true
	}

	if _, err := loadStoredOAuthToken(ctx, resolvedEndpoint); err == nil {
		return true
	}

	return false
}

func printLoginProblem(out io.Writer, problem string) {
	fmt.Fprintf(out, "❌ Problem: %s\n", problem)
}

func loginAccessTokenMessage(endpoint string) string {
	return fmt.Sprintf("\n"+`🛠  To fix: Create an access token by going to %s/user/settings/tokens, then set the following environment variables in your terminal:

   export SRC_ENDPOINT=%s
   export SRC_ACCESS_TOKEN=(your access token)

   To verify that it's working, run the login command again.

   Alternatively, you can try logging in using OAuth by running: src login --oauth %s
`, endpoint, endpoint, endpoint)
}
