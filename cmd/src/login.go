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

		if cfg.configFilePath != "" {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintf(os.Stderr, "⚠️  Warning: Configuring src with a JSON file is deprecated. Please migrate to using the env vars SRC_ENDPOINT, SRC_ACCESS_TOKEN, and SRC_PROXY instead, and then remove %s. See https://github.com/sourcegraph/src-cli#readme for more information.\n", cfg.configFilePath)
		}

		if flagSet.NArg() >= 1 {
			arg := flagSet.Arg(0)
			loginEndpointURL, err := parseEndpoint(arg)
			if err != nil {
				return cmderrors.Usage(fmt.Sprintf("invalid endpoint URL: %s", arg))
			}

			hasEndpointURLConflict := cfg.endpointURL.String() != loginEndpointURL.String()

			if hasEndpointURLConflict {
				// If the default is configured it means SRC_ENDPOINT is not set
				if DefaultEndpointConfigured(cfg) {
					fmt.Fprintf(os.Stderr, "⚠️  Warning: No SRC_ENDPOINT is configured in the environment. Logging in using %q.\n", loginEndpointURL)
					fmt.Fprintf(os.Stderr, "\n💡 Tip: To use this endpoint in your shell, run:\n\n   export SRC_ENDPOINT=%s\n\nNOTE: By default src will use %q if SRC_ENDPOINT is not set.\n", loginEndpointURL, SGDotComEndpoint)
				} else {
					fmt.Fprintf(os.Stderr, "⚠️  Warning: Logging into %s instead of the configured endpoint %s.\n", loginEndpointURL, cfg.endpointURL)
					fmt.Fprintf(os.Stderr, "\n💡 Tip: To use this endpoint in your shell, run:\n\n   export SRC_ENDPOINT=%s\n\n", loginEndpointURL)
				}
			}

			// An explicit endpoint on the CLI overrides the configured endpoint for this login.
			cfg.endpointURL = loginEndpointURL
		}

		client := cfg.apiClient(apiFlags, io.Discard)

		return loginCmd(context.Background(), loginParams{
			cfg:         cfg,
			client:      client,
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
	if err := p.cfg.requireCIAccessToken(); err != nil {
		return err
	}

	_, flow := selectLoginFlow(p)
	if err := flow(ctx, p); err != nil {
		return err
	}
	return nil
}

// selectLoginFlow decides what login flow to run based on configured AuthMode.
func selectLoginFlow(p loginParams) (loginFlowKind, loginFlow) {
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
