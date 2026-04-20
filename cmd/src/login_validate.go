package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func runValidatedLogin(ctx context.Context, p loginParams) error {
	return validateCurrentUser(ctx, p.client, p.out, p.cfg.endpointURL)
}

func validateCurrentUser(ctx context.Context, client api.Client, out io.Writer, endpointURL *url.URL) error {
	query := `query CurrentUser { currentUser { username } }`
	var result struct {
		CurrentUser *struct{ Username string }
	}
	if _, err := client.NewRequest(query, nil).Do(ctx, &result); err != nil {
		if strings.HasPrefix(err.Error(), "error: 401 Unauthorized") || strings.HasPrefix(err.Error(), "error: 403 Forbidden") {
			printLoginProblem(out, "Invalid access token.")
		} else {
			printLoginProblem(out, fmt.Sprintf("Error communicating with %s: %s", endpointURL, err))
		}
		fmt.Fprintln(out, loginAccessTokenMessage(endpointURL))
		fmt.Fprintln(out, "   (If you need to supply custom HTTP request headers, see information about SRC_HEADER_* and SRC_HEADERS env vars at https://github.com/sourcegraph/src-cli/blob/main/AUTH_PROXY.md)")
		return cmderrors.ExitCode1
	}

	if result.CurrentUser == nil {
		// This should never happen; we verified there is an access token, so there should always be
		// a user.
		printLoginProblem(out, fmt.Sprintf("Unable to determine user on %s.", endpointURL))
		return cmderrors.ExitCode1
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "✔︎ Authenticated as %s on %s\n", result.CurrentUser.Username, endpointURL)
	fmt.Fprintln(out)
	return nil
}
