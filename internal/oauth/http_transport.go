package oauth

import (
	"context"
	"net/http"
	"sync"
	"time"
)

var _ http.Transport

var _ http.RoundTripper = (*Transport)(nil)

type Transport struct {
	Base  http.RoundTripper
	Token *Token

	mu sync.Mutex
}

// storeRefreshedTokenFn is the function the transport should use to persist the token - mainly used during
// tests to swap out the implementation out with a mock
var storeRefreshedTokenFn = StoreToken

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	if err := t.refreshToken(ctx); err != nil {
		return nil, err
	}

	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+t.Token.AccessToken)

	if t.Base != nil {
		return t.Base.RoundTrip(req2)
	}
	return http.DefaultTransport.RoundTrip(req2)
}

// refreshToken checks if the token has expired or expiring soon and refreshes it. Once the token is
// refreshed, the in-memory token is updated and a best effort is made to store the token.
// If storing the token fails, no error is returned.
func (t *Transport) refreshToken(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	prevToken := t.Token
	token, err := maybeRefresh(ctx, t.Token)
	if err != nil {
		return err
	}
	t.Token = token
	if token != prevToken {
		// try to save the token if we fail let the request continue with in memory token
		_ = storeRefreshedTokenFn(ctx, token)
	}

	return nil
}

// maybeRefresh conditionally refreshes the token. If the token has expired or is expriing in the next 30s
// it will be refreshed and the updated token will be returned. Otherwise, no refresh occurs and the original
// token is returned.
func maybeRefresh(ctx context.Context, token *Token) (*Token, error) {
	// token has NOT expired and is NOT about to expire in 30s
	if !(token.HasExpired() || token.ExpiringIn(time.Duration(30)*time.Second)) {
		return token, nil
	}
	client := NewClient(token.ClientID)

	resp, err := client.Refresh(ctx, token)
	if err != nil {
		return nil, err
	}

	next := resp.Token(token.Endpoint)
	next.ClientID = token.ClientID
	return next, nil
}

// IsOAuthTransport checks wether the underlying type of the given RoundTripper is a OAuthTransport
func IsOAuthTransport(trp http.RoundTripper) bool {
	_, ok := trp.(*Transport)
	return ok
}
