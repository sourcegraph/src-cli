package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/oauth"
)

func runOAuthLogin(ctx context.Context, p loginParams) error {
	endpointArg := cleanEndpoint(p.endpoint)
	client, err := oauthLoginClient(ctx, p, endpointArg)
	if err != nil {
		printLoginProblem(p.out, fmt.Sprintf("OAuth Device flow authentication failed: %s", err))
		fmt.Fprintln(p.out, loginAccessTokenMessage(endpointArg))
		return cmderrors.ExitCode1
	}

	if err := validateCurrentUser(ctx, client, p.out, endpointArg); err != nil {
		return err
	}

	fmt.Fprintln(p.out)
	fmt.Fprint(p.out, "✔︎ Authenticated with OAuth credentials")
	fmt.Fprintln(p.out)
	return nil
}

func oauthLoginClient(ctx context.Context, p loginParams, endpoint string) (api.Client, error) {
	token, err := runOAuthDeviceFlow(ctx, endpoint, p.out, p.oauthClient)
	if err != nil {
		return nil, err
	}

	if err := oauth.StoreToken(ctx, token); err != nil {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "⚠️  Warning: Failed to store token in keyring store: %q. Continuing with this session only.\n", err)
	}

	return api.NewClient(api.ClientOpts{
		Endpoint:          endpoint,
		AdditionalHeaders: p.cfg.AdditionalHeaders,
		Flags:             p.apiFlags,
		Out:               p.out,
		ProxyURL:          p.cfg.ProxyURL,
		ProxyPath:         p.cfg.ProxyPath,
		OAuthToken:        token,
	}), nil
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
	fmt.Fprint(out, "Waiting for authorization... ")
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
