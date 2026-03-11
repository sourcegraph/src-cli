package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
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

		var loginEndpointURL *url.URL
		if flagSet.NArg() >= 1 {
			arg := flagSet.Arg(0)
			u, err := parseEndpoint(arg)
			if err != nil {
				return cmderrors.Usage(fmt.Sprintf("invalid endpoint URL: %s", arg))
			}
			loginEndpointURL = u
		}

		client := cfg.apiClient(apiFlags, io.Discard)

		return loginCmd(context.Background(), loginParams{
			cfg:              cfg,
			client:           client,
			out:              os.Stdout,
			apiFlags:         apiFlags,
			oauthClient:      oauth.NewClient(oauth.DefaultClientID),
			loginEndpointURL: loginEndpointURL,
		})
	}

	commands = append(commands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

type loginParams struct {
	cfg              *config
	client           api.Client
	out              io.Writer
	apiFlags         *api.Flags
	oauthClient      oauth.Client
	loginEndpointURL *url.URL
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
	if p.cfg.configFilePath != "" {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "⚠️  Warning: Configuring src with a JSON file is deprecated. Please migrate to using the env vars SRC_ENDPOINT, SRC_ACCESS_TOKEN, and SRC_PROXY instead, and then remove %s. See https://github.com/sourcegraph/src-cli#readme for more information.\n", p.cfg.configFilePath)
	}

	_, flow := selectLoginFlow(p)
	return flow(ctx, p)
}

// selectLoginFlow decides what login flow to run based on configured AuthMode.
func selectLoginFlow(p loginParams) (loginFlowKind, loginFlow) {
	if p.loginEndpointURL != nil && p.loginEndpointURL.String() != p.cfg.endpointURL.String() {
		return loginFlowEndpointConflict, runEndpointConflictLogin
	}
	switch p.cfg.AuthMode() {
	case AuthModeOAuth:
		return loginFlowOAuth, runOAuthLogin
	case AuthModeAccessToken:
		return loginFlowValidate, runValidatedLogin
	default:
		return loginFlowMissingAuth, runMissingAuthLogin
	}
}

func printLoginProblem(out io.Writer, problem string) {
	fmt.Fprintf(out, "❌ Problem: %s\n", problem)
}

func loginAccessTokenMessage(endpointURL *url.URL) string {
	return fmt.Sprintf("\n"+`🛠  To fix: Create an access token by going to %s/user/settings/tokens, then set the following environment variables in your terminal:

   export SRC_ENDPOINT=%s
   export SRC_ACCESS_TOKEN=(your access token)

   To verify that it's working, run the login command again.

   Alternatively, you can try logging in interactively by running: src login %s
`, endpointURL, endpointURL, endpointURL)
}
