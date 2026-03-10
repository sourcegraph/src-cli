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

  If no access token is configured, 'src login' uses OAuth device flow automatically:

    $ src login https://sourcegraph.com
`

	flagSet := flag.NewFlagSet("login", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintln(flag.CommandLine.Output(), usage)
		flagSet.PrintDefaults()
	}

	var (
		apiFlags = api.NewFlags(flagSet)
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

func loginCmd(ctx context.Context, p loginParams) error {
	if p.cfg.ConfigFilePath != "" {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "⚠️  Warning: Configuring src with a JSON file is deprecated. Please migrate to using the env vars SRC_ENDPOINT, SRC_ACCESS_TOKEN, and SRC_PROXY instead, and then remove %s. See https://github.com/sourcegraph/src-cli#readme for more information.\n", p.cfg.ConfigFilePath)
	}

	_, flow := selectLoginFlow(ctx, p)
	return flow(ctx, p)
}

// selectLoginFlow decides what login flow to run based on configured AuthMode.
func selectLoginFlow(_ context.Context, p loginParams) (loginFlowKind, loginFlow) {
	endpointArg := cleanEndpoint(p.endpoint)

	switch p.cfg.AuthMode() {
	case AuthModeOAuth:
		return loginFlowOAuth, runOAuthLogin
	case AuthModeAccessToken:
		if endpointArg != p.cfg.Endpoint {
			return loginFlowEndpointConflict, runEndpointConflictLogin
		}
		return loginFlowValidate, runValidatedLogin
	default:
		return loginFlowMissingAuth, runMissingAuthLogin
	}
}

func printLoginProblem(out io.Writer, problem string) {
	fmt.Fprintf(out, "❌ Problem: %s\n", problem)
}

func loginAccessTokenMessage(endpoint string) string {
	return fmt.Sprintf("\n"+`🛠  To fix: Create an access token by going to %s/user/settings/tokens, then set the following environment variables in your terminal:

   export SRC_ENDPOINT=%s
   export SRC_ACCESS_TOKEN=(your access token)

   To verify that it's working, run the login command again.

   Alternatively, you can try logging in interactively by running: src login %s
`, endpoint, endpoint, endpoint)
}
