package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

func TestLogin(t *testing.T) {
	check := func(t *testing.T, cfg *config, endpointArg string) (output string, err error) {
		t.Helper()

		var out bytes.Buffer
		err = loginCmd(context.Background(), cfg, cfg.apiClient(nil, io.Discard), endpointArg, &out)
		return strings.TrimSpace(out.String()), err
	}

	t.Run("different endpoint in config vs. arg", func(t *testing.T) {
		out, err := check(t, &config{Endpoint: "https://example.com"}, "https://sourcegraph.example.com")
		if err != cmderrors.ExitCode1 {
			t.Fatal(err)
		}
		wantOut := "❌ Problem: No access token is configured.\n\n🛠  To fix: Create an access token by going to https://sourcegraph.example.com/user/settings/tokens, then set the following environment variables in your terminal:\n\n   export SRC_ENDPOINT=https://sourcegraph.example.com\n   export SRC_ACCESS_TOKEN=(your access token)\n\n   To verify that it's working, run the login command again."
		if out != wantOut {
			t.Errorf("got output %q, want %q", out, wantOut)
		}
	})

	t.Run("no access token", func(t *testing.T) {
		out, err := check(t, &config{Endpoint: "https://example.com"}, "https://sourcegraph.example.com")
		if err != cmderrors.ExitCode1 {
			t.Fatal(err)
		}
		wantOut := "❌ Problem: No access token is configured.\n\n🛠  To fix: Create an access token by going to https://sourcegraph.example.com/user/settings/tokens, then set the following environment variables in your terminal:\n\n   export SRC_ENDPOINT=https://sourcegraph.example.com\n   export SRC_ACCESS_TOKEN=(your access token)\n\n   To verify that it's working, run the login command again."
		if out != wantOut {
			t.Errorf("got output %q, want %q", out, wantOut)
		}
	})

	t.Run("warning when using config file", func(t *testing.T) {
		out, err := check(t, &config{Endpoint: "https://example.com", ConfigFilePath: "f"}, "https://example.com")
		if err != cmderrors.ExitCode1 {
			t.Fatal(err)
		}
		wantOut := "⚠️  Warning: Configuring src with a JSON file is deprecated. Please migrate to using the env vars SRC_ENDPOINT, SRC_ACCESS_TOKEN, and SRC_PROXY instead, and then remove f. See https://github.com/sourcegraph/src-cli#readme for more information.\n\n❌ Problem: No access token is configured.\n\n🛠  To fix: Create an access token by going to https://example.com/user/settings/tokens, then set the following environment variables in your terminal:\n\n   export SRC_ENDPOINT=https://example.com\n   export SRC_ACCESS_TOKEN=(your access token)\n\n   To verify that it's working, run the login command again."
		if out != wantOut {
			t.Errorf("got output %q, want %q", out, wantOut)
		}
	})

	t.Run("invalid access token", func(t *testing.T) {
		// Dummy HTTP server to return HTTP 401/403.
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "", http.StatusUnauthorized)
		}))
		defer s.Close()

		endpoint := s.URL
		u, _ := url.ParseRequestURI(endpoint)
		out, err := check(t, &config{Endpoint: endpoint, EndpointURL: u, AccessToken: "x"}, endpoint)
		if err != cmderrors.ExitCode1 {
			t.Fatal(err)
		}
		wantOut := "❌ Problem: Invalid access token.\n\n🛠  To fix: Create an access token by going to $ENDPOINT/user/settings/tokens, then set the following environment variables in your terminal:\n\n   export SRC_ENDPOINT=$ENDPOINT\n   export SRC_ACCESS_TOKEN=(your access token)\n\n   To verify that it's working, run the login command again.\n\n   (If you need to supply custom HTTP request headers, see information about SRC_HEADER_* and SRC_HEADERS env vars at https://github.com/sourcegraph/src-cli/blob/main/AUTH_PROXY.md)"
		wantOut = strings.ReplaceAll(wantOut, "$ENDPOINT", endpoint)
		if out != wantOut {
			t.Errorf("got output %q, want %q", out, wantOut)
		}
	})

	t.Run("valid", func(t *testing.T) {
		// Dummy HTTP server to return JSON response with currentUser.
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, `{"data":{"currentUser":{"username":"alice"}}}`)
		}))
		defer s.Close()

		endpoint := s.URL
		u, _ := url.ParseRequestURI(endpoint)
		out, err := check(t, &config{Endpoint: endpoint, EndpointURL: u, AccessToken: "x"}, endpoint)
		if err != nil {
			t.Fatal(err)
		}
		wantOut := "✔️  Authenticated as alice on $ENDPOINT"
		wantOut = strings.ReplaceAll(wantOut, "$ENDPOINT", endpoint)
		if out != wantOut {
			t.Errorf("got output %q, want %q", out, wantOut)
		}
	})
}
