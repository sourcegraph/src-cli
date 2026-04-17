package main

import (
	"context"
	"fmt"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/oauth"
	"github.com/urfave/cli/v3"
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

const authTokenExamples = `
Print the current authentication token.

Use --header to print a complete Authorization header instead.

Examples:

Raw token output:

$ src auth token
sgp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

Authorization header output:

$ src auth token --header
Authorization: token sgp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

If you are authenticated with OAuth instead of SRC_ACCESS_TOKEN, the header uses the Bearer scheme:

$ src auth token --header
Authorization: Bearer eyJhbGciOi...
`

var authTokenCommand = clicompat.Wrap(&cli.Command{
	Name:        "token",
	Usage:       "prints the current authentication token or Authorization header",
	UsageText:   "src auth token [options]",
	Description: authTokenExamples,
	HideVersion: true,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "header",
			Usage: "print the token as an Authorization header",
		},
	},
	Action: func(ctx context.Context, cmd *cli.Command) error {
		token, err := resolveAuthToken(ctx, cfg)
		if err != nil {
			return err
		}

		token = formatAuthTokenOutput(token, cfg.AuthMode(), cmd.Bool("header"))
		_, err = fmt.Fprintln(cmd.Writer, token)
		return err
	},
})

func resolveAuthToken(ctx context.Context, cfg *config) (string, error) {
	if err := cfg.requireCIAccessToken(); err != nil {
		return "", err
	}

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
