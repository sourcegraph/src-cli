package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/oauth"
)

var (
	loadOAuthToken         = oauth.LoadToken
	newOAuthTokenRefresher = func(token *oauth.Token) oauthTokenRefresher {
		return oauth.NewTokenRefresher(token)
	}
)

type oauthTokenRefresher interface {
	GetToken(ctx context.Context) (oauth.Token, error)
}

func init() {
	flagSet := flag.NewFlagSet("token", flag.ExitOnError)
	header := flagSet.Bool("header", false, "print the token as an Authorization header")
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src auth token':\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Print the current authentication token.\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Use --header to print a complete Authorization header instead.\n\n")
		flagSet.PrintDefaults()
	}

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		token, err := resolveAuthToken(context.Background(), cfg)
		if err != nil {
			return err
		}

		token = formatAuthTokenOutput(token, cfg.AuthMode(), *header)
		fmt.Println(token)
		return nil
	}

	authCommands = append(authCommands, &command{
		flagSet:   flagSet,
		handler:   handler,
		usageFunc: usageFunc,
	})
}

func resolveAuthToken(ctx context.Context, cfg *config) (string, error) {
	if cfg.accessToken != "" {
		return cfg.accessToken, nil
	}

	oauthToken, err := loadOAuthToken(ctx, cfg.endpointURL)
	if err != nil {
		return "", errors.Wrap(err, "error loading OAuth token; set SRC_ACCESS_TOKEN or run `src login`")
	}

	token, err := newOAuthTokenRefresher(oauthToken).GetToken(ctx)
	if err != nil {
		return "", errors.Wrap(err, "refreshing OAuth token")
	}

	return token.AccessToken, nil
}

func formatAuthTokenOutput(token string, mode AuthMode, header bool) string {
	if !header {
		return token
	}

	if mode == AuthModeAccessToken {
		return fmt.Sprintf("Authorization: token %s", token)
	}

	return fmt.Sprintf("Authorization: Bearer %s", token)
}
