package main

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/sourcegraph/src-cli/internal/oauth"
)

func TestResolveAuthToken(t *testing.T) {
	t.Run("uses configured access token before keyring", func(t *testing.T) {
		reset := stubAuthTokenDependencies(t)
		defer reset()

		newRefresherCalled := false
		newOAuthTokenRefresher = func(*oauth.Token) oauthTokenRefresher {
			newRefresherCalled = true
			return fakeOAuthTokenRefresher{}
		}

		token, err := resolveAuthToken(context.Background(), &config{
			accessToken: "access-token",
			endpointURL: mustParseURL(t, "https://example.com"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if token != "access-token" {
			t.Fatalf("token = %q, want %q", token, "access-token")
		}
		if newRefresherCalled {
			t.Fatal("expected OAuth token refresher not to be created")
		}
	})

	t.Run("requires access token in CI", func(t *testing.T) {
		reset := stubAuthTokenDependencies(t)
		defer reset()

		loadCalled := false
		loadOAuthToken = func(context.Context, *url.URL) (*oauth.Token, error) {
			loadCalled = true
			return nil, nil
		}

		_, err := resolveAuthToken(context.Background(), &config{
			inCI:        true,
			endpointURL: mustParseURL(t, "https://example.com"),
		})
		if err != errCIAccessTokenRequired {
			t.Fatalf("err = %v, want %v", err, errCIAccessTokenRequired)
		}
		if loadCalled {
			t.Fatal("expected OAuth token loader not to be called")
		}
	})

	t.Run("uses stored oauth token", func(t *testing.T) {
		reset := stubAuthTokenDependencies(t)
		defer reset()

		loadOAuthToken = func(context.Context, *url.URL) (*oauth.Token, error) {
			return &oauth.Token{
				AccessToken: "oauth-token",
			}, nil
		}

		newOAuthTokenRefresher = func(*oauth.Token) oauthTokenRefresher {
			return fakeOAuthTokenRefresher{token: oauth.Token{AccessToken: "oauth-token"}}
		}

		token, err := resolveAuthToken(context.Background(), &config{
			endpointURL: mustParseURL(t, "https://example.com"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if token != "oauth-token" {
			t.Fatalf("token = %q, want %q", token, "oauth-token")
		}
	})

	t.Run("refreshes expiring oauth token", func(t *testing.T) {
		reset := stubAuthTokenDependencies(t)
		defer reset()

		loadOAuthToken = func(context.Context, *url.URL) (*oauth.Token, error) {
			return &oauth.Token{AccessToken: "old-token"}, nil
		}

		newOAuthTokenRefresher = func(*oauth.Token) oauthTokenRefresher {
			return fakeOAuthTokenRefresher{token: oauth.Token{AccessToken: "new-token"}}
		}

		token, err := resolveAuthToken(context.Background(), &config{
			endpointURL: mustParseURL(t, "https://example.com"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if token != "new-token" {
			t.Fatalf("token = %q, want %q", token, "new-token")
		}
	})

	t.Run("returns refresh error when shared refresh logic fails", func(t *testing.T) {
		reset := stubAuthTokenDependencies(t)
		defer reset()

		loadOAuthToken = func(context.Context, *url.URL) (*oauth.Token, error) {
			return &oauth.Token{AccessToken: "old-token"}, nil
		}
		newOAuthTokenRefresher = func(*oauth.Token) oauthTokenRefresher {
			return fakeOAuthTokenRefresher{err: fmt.Errorf("refresh failed")}
		}

		_, err := resolveAuthToken(context.Background(), &config{
			endpointURL: mustParseURL(t, "https://example.com"),
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestFormatAuthTokenOutput(t *testing.T) {
	tests := []struct {
		name   string
		token  string
		mode   AuthMode
		header bool
		want   string
	}{
		{
			name:   "raw access token",
			token:  "access-token",
			mode:   AuthModeAccessToken,
			header: false,
			want:   "access-token",
		},
		{
			name:   "raw oauth token",
			token:  "oauth-token",
			mode:   AuthModeOAuth,
			header: false,
			want:   "oauth-token",
		},
		{
			name:   "authorization header for access token",
			token:  "access-token",
			mode:   AuthModeAccessToken,
			header: true,
			want:   "Authorization: token access-token",
		},
		{
			name:   "authorization header for oauth token",
			token:  "oauth-token",
			mode:   AuthModeOAuth,
			header: true,
			want:   "Authorization: Bearer oauth-token",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := formatAuthTokenOutput(test.token, test.mode, test.header); got != test.want {
				t.Fatalf("formatAuthTokenOutput(%q, %v, %v) = %q, want %q", test.token, test.mode, test.header, got, test.want)
			}
		})
	}
}

func stubAuthTokenDependencies(t *testing.T) func() {
	t.Helper()

	prevLoad := loadOAuthToken
	prevNewRefresher := newOAuthTokenRefresher

	return func() {
		loadOAuthToken = prevLoad
		newOAuthTokenRefresher = prevNewRefresher
	}
}

type fakeOAuthTokenRefresher struct {
	token oauth.Token
	err   error
}

func (r fakeOAuthTokenRefresher) GetToken(context.Context) (oauth.Token, error) {
	if r.err != nil {
		return oauth.Token{}, r.err
	}
	return r.token, nil
}
