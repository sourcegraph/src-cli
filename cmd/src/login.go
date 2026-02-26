package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

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
			cfg:              cfg,
			client:           client,
			endpoint:         endpoint,
			out:              os.Stdout,
			useOAuth:         *useOAuth,
			apiFlags:         apiFlags,
			deviceFlowClient: oauth.NewClient(oauth.DefaultClientID),
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
	useOAuth         bool
	apiFlags         *api.Flags
	deviceFlowClient oauth.Client
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

	cfg.Endpoint = endpointArg

	if p.useOAuth {
		token, err := runOAuthDeviceFlow(ctx, endpointArg, out, p.deviceFlowClient)
		if err != nil {
			printProblem(fmt.Sprintf("OAuth Device flow authentication failed: %s", err))
			fmt.Fprintln(out, createAccessTokenMessage)
			return cmderrors.ExitCode1
		}

		if err := oauth.StoreToken(ctx, token); err != nil {
			fmt.Fprintln(out)
			fmt.Fprintf(out, "⚠️  Warning: Failed to store token in keyring store: %q. Continuing with this session only.\n", err)
		}

		client = api.NewClient(api.ClientOpts{
			Endpoint:          cfg.Endpoint,
			AdditionalHeaders: cfg.AdditionalHeaders,
			Flags:             p.apiFlags,
			Out:               out,
			ProxyURL:          cfg.ProxyURL,
			ProxyPath:         cfg.ProxyPath,
			OAuthToken:        token,
		})
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

	if p.useOAuth {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Authenticated with OAuth credentials")
	}

	fmt.Fprintln(out)
	return nil
}

func runOAuthDeviceFlow(ctx context.Context, endpoint string, out io.Writer, client oauth.Client) (*oauth.Token, error) {
	authResp, err := client.Start(ctx, endpoint, nil)
	if err != nil {
		return nil, err
	}

	authURL := authResp.VerificationURIComplete
	msg := fmt.Sprintf("If your browser did not open automatically, visit %s.", authURL)
	if authURL == "" {
		authURL = authResp.VerificationURI
		msg = fmt.Sprintf("If your browser did not open automatically, visit %s and enter the user code %s", authURL, authResp.DeviceCode)
	}
	_ = openInBrowser(authURL)
	fmt.Fprintln(out)
	fmt.Fprint(out, msg)

	fmt.Fprintln(out)
	fmt.Fprint(out, "Waiting for authorization...")
	defer fmt.Fprintf(out, "DONE\n\n")

	interval := time.Duration(authResp.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}

	resp, err := client.Poll(ctx, endpoint, authResp.DeviceCode, interval, authResp.ExpiresIn)
	if err != nil {
		return nil, err
	}

	token := resp.Token(endpoint)
	token.ClientID = client.ClientID()
	return token, nil
}

func openInBrowser(url string) error {
	if url == "" {
		return nil
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// "start" is a cmd.exe built-in; the empty string is the window title.
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Run()
}
