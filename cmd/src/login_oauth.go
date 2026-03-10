package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/oauth"
)

var loadStoredOAuthToken = oauth.LoadToken

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

// oauthLoginClient returns a api.Client with the OAuth token set. It will check secret storage for a token
// and use it if one is present.
// If no token is found, it will start a OAuth Device flow to get a token and storage in secret storage.
func oauthLoginClient(ctx context.Context, p loginParams, endpoint string) (api.Client, error) {
	// if we have a stored token, used it. Otherwise run the device flow
	if token, err := loadStoredOAuthToken(ctx, endpoint); err == nil {
		return newOAuthAPIClient(p, endpoint, token), nil
	}

	token, err := runOAuthDeviceFlow(ctx, endpoint, p.out, p.oauthClient)
	if err != nil {
		return nil, err
	}

	if err := oauth.StoreToken(ctx, token); err != nil {
		fmt.Fprintln(p.out)
		fmt.Fprintf(p.out, "⚠️  Warning: Failed to store token in keyring store: %q. Continuing with this session only.\n", err)
	}

	return newOAuthAPIClient(p, endpoint, token), nil
}

func newOAuthAPIClient(p loginParams, endpoint string, token *oauth.Token) api.Client {
	return api.NewClient(api.ClientOpts{
		Endpoint:          endpoint,
		AdditionalHeaders: p.cfg.AdditionalHeaders,
		Flags:             p.apiFlags,
		Out:               p.out,
		ProxyURL:          p.cfg.ProxyURL,
		ProxyPath:         p.cfg.ProxyPath,
		OAuthToken:        token,
	})
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

// validateBrowserURL checks that rawURL is a valid HTTP(S) URL to prevent
// command injection via malicious URLs returned by an OAuth server.
func validateBrowserURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q: only http and https are allowed", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("URL has no host")
	}
	return nil
}

func openInBrowser(rawURL string) error {
	if rawURL == "" {
		return nil
	}

	if err := validateBrowserURL(rawURL); err != nil {
		return err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Run()
}
