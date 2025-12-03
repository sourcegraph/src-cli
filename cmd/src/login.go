package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/oauthdevice"
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

    $ src login --device-flow https://sourcegraph.com


  Override the default client id used during device flow when authenticating:

    $ src login --device-flow https://sourcegraph.com --client-id sgo_my_own_client_id
`

	flagSet := flag.NewFlagSet("login", flag.ExitOnError)
	usageFunc := func() {
		fmt.Fprintln(flag.CommandLine.Output(), usage)
		flagSet.PrintDefaults()
	}

	var (
		apiFlags      = api.NewFlags(flagSet)
		useDeviceFlow = flagSet.Bool("device-flow", false, "Use OAuth device flow to obtain an access token interactively")
		OAuthClientID = flagSet.String("client-id", oauthdevice.DefaultClientID, "Client ID to use with OAuth device flow. Will use the predefined src cli client ID if not specified.")
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

		if *OAuthClientID == "" {
			return cmderrors.Usage("no value specified for client-id")
		}

		client := cfg.apiClient(apiFlags, io.Discard)

		return loginCmd(context.Background(), loginParams{
			cfg:              cfg,
			client:           client,
			endpoint:         endpoint,
			out:              os.Stdout,
			useDeviceFlow:    *useDeviceFlow,
			apiFlags:         apiFlags,
			deviceFlowClient: oauthdevice.NewClient(*OAuthClientID),
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
	endpoint         string
	out              io.Writer
	useDeviceFlow    bool
	apiFlags         *api.Flags
	deviceFlowClient oauthdevice.Client
}

func loginCmd(ctx context.Context, p loginParams) error {
	endpointArg := cleanEndpoint(p.endpoint)
	cfg := p.cfg
	client := p.client
	out := p.out

	printProblem := func(problem string) {
		fmt.Fprintf(out, "❌ Problem: %s\n", problem)
	}

	createAccessTokenMessage := fmt.Sprintf("\n"+`🛠  To fix: Create an access token by going to %s/user/settings/tokens, then set the following environment variables in your terminal:

   export SRC_ENDPOINT=%s
   export SRC_ACCESS_TOKEN=(your access token)

   To verify that it's working, run the login command again.
`, endpointArg, endpointArg)

	if cfg.ConfigFilePath != "" {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "⚠️  Warning: Configuring src with a JSON file is deprecated. Please migrate to using the env vars SRC_ENDPOINT, SRC_ACCESS_TOKEN, and SRC_PROXY instead, and then remove %s. See https://github.com/sourcegraph/src-cli#readme for more information.\n", cfg.ConfigFilePath)
	}

	noToken := cfg.AccessToken == ""
	endpointConflict := endpointArg != cfg.Endpoint

	if p.useDeviceFlow {
		token, err := runDeviceFlow(ctx, endpointArg, out, p.deviceFlowClient)
		if err != nil {
			printProblem(fmt.Sprintf("Device flow authentication failed: %s", err))
			fmt.Fprintln(out, createAccessTokenMessage)
			return cmderrors.ExitCode1
		}

		cfg.AccessToken = token
		cfg.Endpoint = endpointArg
		client = cfg.apiClient(p.apiFlags, out)
	} else if noToken || endpointConflict {
		fmt.Fprintln(out)
		switch {
		case noToken:
			printProblem("No access token is configured.")
		case endpointConflict:
			printProblem(fmt.Sprintf("The configured endpoint is %s, not %s.", cfg.Endpoint, endpointArg))
		}
		fmt.Fprintln(out, createAccessTokenMessage)
		return cmderrors.ExitCode1
	}

	// See if the user is already authenticated.
	query := `query CurrentUser { currentUser { username } }`
	var result struct {
		CurrentUser *struct{ Username string }
	}
	if _, err := client.NewRequest(query, nil).Do(ctx, &result); err != nil {
		if strings.HasPrefix(err.Error(), "error: 401 Unauthorized") || strings.HasPrefix(err.Error(), "error: 403 Forbidden") {
			printProblem("Invalid access token.")
		} else {
			printProblem(fmt.Sprintf("Error communicating with %s: %s", endpointArg, err))
		}
		fmt.Fprintln(out, createAccessTokenMessage)
		fmt.Fprintln(out, "   (If you need to supply custom HTTP request headers, see information about SRC_HEADER_* and SRC_HEADERS env vars at https://github.com/sourcegraph/src-cli/blob/main/AUTH_PROXY.md)")
		return cmderrors.ExitCode1
	}

	if result.CurrentUser == nil {
		// This should never happen; we verified there is an access token, so there should always be
		// a user.
		printProblem(fmt.Sprintf("Unable to determine user on %s.", endpointArg))
		return cmderrors.ExitCode1
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "✔️  Authenticated as %s on %s\n", result.CurrentUser.Username, endpointArg)

	if p.useDeviceFlow {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "To use this access token, set the following environment variables in your terminal:\n\n")
		fmt.Fprintf(out, "   export SRC_ENDPOINT=%s\n", endpointArg)
		fmt.Fprintf(out, "   export SRC_ACCESS_TOKEN=%s\n", cfg.AccessToken)
	}

	fmt.Fprintln(out)
	return nil
}

func runDeviceFlow(ctx context.Context, endpoint string, out io.Writer, client oauthdevice.Client) (string, error) {
	authResp, err := client.Start(ctx, endpoint, nil)
	if err != nil {
		return "", err
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "To authenticate, visit %s and enter the code: %s\n", authResp.VerificationURI, authResp.UserCode)
	if authResp.VerificationURIComplete != "" {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Alternatively, you can open: %s\n", authResp.VerificationURIComplete)
	}
	fmt.Fprintln(out)
	fmt.Fprint(out, "Waiting for authorization...")
	defer fmt.Fprintf(out, "DONE\n\n")

	interval := time.Duration(authResp.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}

	tokenResp, err := client.Poll(ctx, endpoint, authResp.DeviceCode, interval, authResp.ExpiresIn)
	if err != nil {
		return "", err
	}

	return tokenResp.AccessToken, nil
}
