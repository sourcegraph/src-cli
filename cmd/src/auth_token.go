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
	usageFunc := func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src auth token':\n")
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
