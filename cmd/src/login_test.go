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
	"time"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/oauth"
	"github.com/urfave/cli/v3"
)

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("failed to parse URL %q: %v", raw, err)
	}
	return u
}

func TestLogin(t *testing.T) {
	check := func(t *testing.T, cfg *config) (output string, err error) {
		t.Helper()

		var out bytes.Buffer
		err = loginCmd(context.Background(), loginParams{
			cfg:         cfg,
			client:      cfg.apiClient(nil, io.Discard),
			out:         &out,
			oauthClient: fakeOAuthClient{startErr: fmt.Errorf("oauth unavailable")},
		})
		return strings.TrimSpace(out.String()), err
	}

	t.Run("no access token triggers oauth flow", func(t *testing.T) {
		u := &url.URL{Scheme: "https", Host: "example.com"}
		out, err := check(t, &config{endpointURL: u})
		if err == nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "OAuth Device flow authentication failed:") {
			t.Errorf("got output %q, want oauth failure output", out)
		}
	})

	t.Run("CI requires access token", func(t *testing.T) {
		u := &url.URL{Scheme: "https", Host: "example.com"}
		out, err := check(t, &config{endpointURL: u, inCI: true})
		if err != errCIAccessTokenRequired {
			t.Fatalf("err = %v, want %v", err, errCIAccessTokenRequired)
		}
		if out != "" {
			t.Fatalf("output = %q, want empty output", out)
		}
	})

	t.Run("invalid access token", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "", http.StatusUnauthorized)
		}))
		defer s.Close()

		u := mustParseURL(t, s.URL)
		out, err := check(t, &config{endpointURL: u, accessToken: "x"})
		if err != cmderrors.ExitCode1 {
			t.Fatal(err)
		}
		wantOut := "❌ Problem: Invalid access token.\n\n🛠  To fix: Create an access token by going to $ENDPOINT/user/settings/tokens, then set the following environment variables in your terminal:\n\n   export SRC_ENDPOINT=$ENDPOINT\n   export SRC_ACCESS_TOKEN=(your access token)\n\n   To verify that it's working, run the login command again.\n\n   Alternatively, you can try logging in interactively by running: src login $ENDPOINT\n\n   (If you need to supply custom HTTP request headers, see information about SRC_HEADER_* and SRC_HEADERS env vars at https://github.com/sourcegraph/src-cli/blob/main/AUTH_PROXY.md)"
		wantOut = strings.ReplaceAll(wantOut, "$ENDPOINT", s.URL)
		if out != wantOut {
			t.Errorf("got output %q, want %q", out, wantOut)
		}
	})

	t.Run("valid", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, `{"data":{"currentUser":{"username":"alice"}}}`)
		}))
		defer s.Close()

		u := mustParseURL(t, s.URL)
		out, err := check(t, &config{endpointURL: u, accessToken: "x"})
		if err != nil {
			t.Fatal(err)
		}
		wantOut := "✔︎ Authenticated as alice on $ENDPOINT"
		wantOut = strings.ReplaceAll(wantOut, "$ENDPOINT", s.URL)
		if out != wantOut {
			t.Errorf("got output %q, want %q", out, wantOut)
		}
	})

	t.Run("reuses stored oauth token before device flow", func(t *testing.T) {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, `{"data":{"currentUser":{"username":"alice"}}}`)
		}))
		defer s.Close()

		restoreStoredOAuthLoader(t, func(_ context.Context, _ *url.URL) (*oauth.Token, error) {
			return &oauth.Token{
				Endpoint:    s.URL,
				ClientID:    oauth.DefaultClientID,
				AccessToken: "oauth-token",
				ExpiresAt:   time.Now().Add(time.Hour),
			}, nil
		})

		u, _ := url.ParseRequestURI(s.URL)
		startCalled := false
		var out bytes.Buffer
		err := loginCmd(context.Background(), loginParams{
			cfg:    &config{endpointURL: u},
			client: (&config{endpointURL: u}).apiClient(nil, io.Discard),
			out:    &out,
			oauthClient: fakeOAuthClient{
				startErr:    fmt.Errorf("unexpected call to Start"),
				startCalled: &startCalled,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if startCalled {
			t.Fatal("expected stored oauth token to avoid device flow")
		}
		gotOut := strings.TrimSpace(out.String())
		wantOut := "✔︎ Authenticated as alice on $ENDPOINT\n\n\n✔︎ Authenticated with OAuth credentials"
		wantOut = strings.ReplaceAll(wantOut, "$ENDPOINT", s.URL)
		if gotOut != wantOut {
			t.Errorf("got output %q, want %q", gotOut, wantOut)
		}
	})
}

func TestLoginCommand(t *testing.T) {
	prevCfg := cfg
	defer func() { cfg = prevCfg }()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"data":{"currentUser":{"username":"alice"}}}`)
	}))
	defer s.Close()

	cfg = &config{
		endpointURL: mustParseURL(t, s.URL),
		accessToken: "x",
	}

	var out bytes.Buffer
	cmd := *loginCommand
	cmd.Writer = &out
	cmd.ErrWriter = &out
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}

	err := cmd.Run(context.Background(), []string{"login", s.URL})
	if err != nil {
		t.Fatal(err)
	}

	want := "\n✔︎ Authenticated as alice on " + s.URL + "\n\n\n💡 Tip: To use this endpoint in your shell, run:\n\n   export SRC_ENDPOINT=" + s.URL + "\n\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

type fakeOAuthClient struct {
	startErr    error
	startCalled *bool
}

func (f fakeOAuthClient) ClientID() string {
	return oauth.DefaultClientID
}

func (f fakeOAuthClient) Discover(context.Context, *url.URL) (*oauth.OIDCConfiguration, error) {
	return nil, fmt.Errorf("unexpected call to Discover")
}

func (f fakeOAuthClient) Start(context.Context, *url.URL, []string) (*oauth.DeviceAuthResponse, error) {
	if f.startCalled != nil {
		*f.startCalled = true
	}
	return nil, f.startErr
}

func (f fakeOAuthClient) Poll(context.Context, *url.URL, string, time.Duration, int) (*oauth.TokenResponse, error) {
	return nil, fmt.Errorf("unexpected call to Poll")
}

func (f fakeOAuthClient) Refresh(context.Context, *oauth.Token) (*oauth.TokenResponse, error) {
	return nil, fmt.Errorf("unexpected call to Refresh")
}

func TestValidateBrowserURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid https", url: "https://example.com/device?code=ABC", wantErr: false},
		{name: "valid http", url: "http://localhost:3080/auth", wantErr: false},
		{name: "command injection ampersand", url: "https://example.com & calc.exe", wantErr: true},
		{name: "command injection pipe", url: "https://x | powershell -enc ZABp", wantErr: true},
		{name: "file scheme", url: "file:///etc/passwd", wantErr: true},
		{name: "javascript scheme", url: "javascript:alert(1)", wantErr: true},
		{name: "empty scheme", url: "://no-scheme", wantErr: true},
		{name: "no host", url: "https://", wantErr: true},
		{name: "relative path", url: "/just/a/path", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBrowserURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBrowserURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

// TestValidateBrowserURL_WindowsRundll32Escape tests that validateBrowserURL blocks
// payloads that could abuse the Windows "rundll32 url.dll,OpenURL" browser opener
// (LOLBAS T1218.011). If any of these cases pass validation, an attacker-controlled
// URL could execute arbitrary files via rundll32.
// Reference: https://lolbas-project.github.io/lolbas/Libraries/Url/
func TestValidateBrowserURL_WindowsRundll32Escape(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		// url.dll OpenURL can launch .hta payloads via mshta.exe
		{name: "hta via file protocol", url: "file:///C:/Temp/payload.hta"},
		// url.dll OpenURL can launch executables from .url shortcut files
		{name: "url shortcut file", url: "file:///C:/Temp/launcher.url"},
		// url.dll OpenURL / FileProtocolHandler can run executables directly
		{name: "exe via file protocol", url: "file:///C:/Windows/System32/calc.exe"},
		// Obfuscated file protocol handler variant
		{name: "obfuscated file handler", url: "file:///C:/Temp/payload.exe"},
		// UNC path via file scheme to remote payload
		{name: "unc path file scheme", url: "file://attacker.com/share/payload.exe"},
		// data: URI could be passed through to a handler
		{name: "data uri", url: "data:text/html,<script>alert(1)</script>"},
		// vbscript scheme
		{name: "vbscript scheme", url: "vbscript:Execute(\"MsgBox(1)\")"},
		// about scheme
		{name: "about scheme", url: "about:blank"},
		// ms-msdt protocol handler (Follina-style)
		{name: "ms-msdt handler", url: "ms-msdt:/id PCWDiagnostic /skip force /param"},
		// search-ms protocol handler
		{name: "search-ms handler", url: "search-ms:query=calc&crumb=location:\\\\attacker.com\\share"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateBrowserURL(tt.url); err == nil {
				t.Errorf("validateBrowserURL(%q) = nil; want error (payload must be blocked to prevent rundll32 url.dll,OpenURL abuse)", tt.url)
			}
		})
	}
}

func restoreStoredOAuthLoader(t *testing.T, loader func(context.Context, *url.URL) (*oauth.Token, error)) {
	t.Helper()

	prev := loadStoredOAuthToken
	loadStoredOAuthToken = loader
	t.Cleanup(func() {
		loadStoredOAuthToken = prev
	})
}
