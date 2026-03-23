package main

import (
	"context"
	"fmt"
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
				AccessToken: "access-token",
				Endpoint:    "https://example.com",
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

	t.Run("uses stored oauth token", func(t *testing.T) {
		reset := stubAuthTokenDependencies(t)
		defer reset()

			loadOAuthToken = func(context.Context, string) (*oauth.Token, error) {
				return &oauth.Token{
					AccessToken: "oauth-token",
				}, nil
		}

		newOAuthTokenRefresher = func(*oauth.Token) oauthTokenRefresher {
			return fakeOAuthTokenRefresher{token: oauth.Token{AccessToken: "oauth-token"}}
			}

			token, err := resolveAuthToken(context.Background(), &config{
				Endpoint: "https://example.com",
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

			loadOAuthToken = func(context.Context, string) (*oauth.Token, error) {
				return &oauth.Token{AccessToken: "old-token"}, nil
			}

		newOAuthTokenRefresher = func(*oauth.Token) oauthTokenRefresher {
			return fakeOAuthTokenRefresher{token: oauth.Token{AccessToken: "new-token"}}
			}

			token, err := resolveAuthToken(context.Background(), &config{
				Endpoint: "https://example.com",
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

			loadOAuthToken = func(context.Context, string) (*oauth.Token, error) {
				return &oauth.Token{AccessToken: "old-token"}, nil
			}
		newOAuthTokenRefresher = func(*oauth.Token) oauthTokenRefresher {
			return fakeOAuthTokenRefresher{err: fmt.Errorf("refresh failed")}
		}

			_, err := resolveAuthToken(context.Background(), &config{
				Endpoint: "https://example.com",
			})
			if err == nil {
				t.Fatal("expected error")
			}
		})
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
