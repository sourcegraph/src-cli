package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/oauth"
)

func TestLogin(t *testing.T) {
	check := func(t *testing.T, cfg *config, endpointArg string) (output string, err error) {
		t.Helper()

		var out bytes.Buffer
		err = loginCmd(context.Background(), loginParams{
			cfg:         cfg,
			client:      cfg.apiClient(nil, io.Discard),
			endpoint:    endpointArg,
			out:         &out,
			oauthClient: fakeOAuthClient{startErr: fmt.Errorf("oauth unavailable")},
		})
		return strings.TrimSpace(out.String()), err
	}

	t.Run("different endpoint in config vs. arg", func(t *testing.T) {
		out, err := check(t, &config{Endpoint: "https://example.com"}, "https://sourcegraph.example.com")
		if err == nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "OAuth Device flow authentication failed:") {
			t.Errorf("got output %q, want oauth failure output", out)
		}
	})

	t.Run("no access token triggers oauth flow", func(t *testing.T) {
		out, err := check(t, &config{Endpoint: "https://example.com"}, "https://sourcegraph.example.com")
		if err == nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "OAuth Device flow authentication failed:") {
			t.Errorf("got output %q, want oauth failure output", out)
		}
	})

	t.Run("warning when using config file", func(t *testing.T) {
		out, err := check(t, &config{Endpoint: "https://example.com", ConfigFilePath: "f"}, "https://example.com")
		if err == nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "Configuring src with a JSON file is deprecated") {
			t.Errorf("got output %q, want deprecation warning", out)
		}
		if !strings.Contains(out, "OAuth Device flow authentication failed:") {
			t.Errorf("got output %q, want oauth failure output", out)
		}
	})

	t.Run("invalid access token", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "", http.StatusUnauthorized)
		}))
		defer s.Close()

		endpoint := s.URL
		out, err := check(t, &config{Endpoint: endpoint, AccessToken: "x"}, endpoint)
		if err != cmderrors.ExitCode1 {
			t.Fatal(err)
		}
		wantOut := "❌ Problem: Invalid access token.\n\n🛠  To fix: Create an access token by going to $ENDPOINT/user/settings/tokens, then set the following environment variables in your terminal:\n\n   export SRC_ENDPOINT=$ENDPOINT\n   export SRC_ACCESS_TOKEN=(your access token)\n\n   To verify that it's working, run the login command again.\n\n   Alternatively, you can try logging in interactively by running: src login $ENDPOINT\n\n   (If you need to supply custom HTTP request headers, see information about SRC_HEADER_* and SRC_HEADERS env vars at https://github.com/sourcegraph/src-cli/blob/main/AUTH_PROXY.md)"
		wantOut = strings.ReplaceAll(wantOut, "$ENDPOINT", endpoint)
		if out != wantOut {
			t.Errorf("got output %q, want %q", out, wantOut)
		}
	})

	t.Run("valid", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, `{"data":{"currentUser":{"username":"alice"}}}`)
		}))
		defer s.Close()

		endpoint := s.URL
		out, err := check(t, &config{Endpoint: endpoint, AccessToken: "x"}, endpoint)
		if err != nil {
			t.Fatal(err)
		}
		wantOut := "✔︎ Authenticated as alice on $ENDPOINT"
		wantOut = strings.ReplaceAll(wantOut, "$ENDPOINT", endpoint)
		if out != wantOut {
			t.Errorf("got output %q, want %q", out, wantOut)
		}
	})
}

type fakeOAuthClient struct {
	startErr error
}

func (f fakeOAuthClient) ClientID() string {
	return oauth.DefaultClientID
}

func (f fakeOAuthClient) Discover(context.Context, string) (*oauth.OIDCConfiguration, error) {
	return nil, fmt.Errorf("unexpected call to Discover")
}

func (f fakeOAuthClient) Start(context.Context, string, []string) (*oauth.DeviceAuthResponse, error) {
	return nil, f.startErr
}

func (f fakeOAuthClient) Poll(context.Context, string, string, time.Duration, int) (*oauth.TokenResponse, error) {
	return nil, fmt.Errorf("unexpected call to Poll")
}

func (f fakeOAuthClient) Refresh(context.Context, *oauth.Token) (*oauth.TokenResponse, error) {
	return nil, fmt.Errorf("unexpected call to Refresh")
}

func TestSelectLoginFlow(t *testing.T) {
	t.Run("uses oauth flow when no access token is configured", func(t *testing.T) {
		params := loginParams{
			cfg:      &config{Endpoint: "https://example.com"},
			endpoint: "https://sourcegraph.example.com",
		}

		if got, _ := selectLoginFlow(context.Background(), params); got != loginFlowOAuth {
			t.Fatalf("flow = %v, want %v", got, loginFlowOAuth)
		}
	})

	t.Run("uses endpoint conflict flow when auth exists for a different endpoint", func(t *testing.T) {
		params := loginParams{
			cfg:      &config{Endpoint: "https://example.com", AccessToken: "x"},
			endpoint: "https://sourcegraph.example.com",
		}

		if got, _ := selectLoginFlow(context.Background(), params); got != loginFlowEndpointConflict {
			t.Fatalf("flow = %v, want %v", got, loginFlowEndpointConflict)
		}
	})

	t.Run("uses validation flow when auth exists for the selected endpoint", func(t *testing.T) {
		params := loginParams{
			cfg:      &config{Endpoint: "https://example.com", AccessToken: "x"},
			endpoint: "https://example.com",
		}

		if got, _ := selectLoginFlow(context.Background(), params); got != loginFlowValidate {
			t.Fatalf("flow = %v, want %v", got, loginFlowValidate)
		}
	})
}
