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
	Base http.RoundTripper
	//Token is a OAuth token (which has a refresh token) that should be used during roundtrip to automatically
	//refresh the OAuth access token once the current one has expired or is soon to expire
	Token *Token

	//mu is a mutex that should be acquired whenever token used
	mu sync.Mutex
}

// storeRefreshedTokenFn is the function the transport should use to persist the token - mainly used during
// tests to swap out the implementation out with a mock
var storeRefreshedTokenFn = StoreToken

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	token, err := t.getToken(ctx)
	if err != nil {
		return nil, err
	}

	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+token.AccessToken)

	if t.Base != nil {
		return t.Base.RoundTrip(req2)
	}
	return http.DefaultTransport.RoundTrip(req2)
}

// getToken returns a value copy of the token. If the token has expired or expiring soon it will be refreshed before returning.
// Once the token is refreshed, the in-memory token is updated and a best effort is made to store the token.
//
// If storing the token fails, no error is returned. An error is only returned if refreshing the token
// fails.
func (t *Transport) getToken(ctx context.Context) (Token, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	prevToken := t.Token
	token, err := maybeRefresh(ctx, t.Token)
	if err != nil {
		return Token{}, err
	}
	t.Token = token
	if token != prevToken {
		// try to save the token if we fail let the request continue with in memory token
		_ = storeRefreshedTokenFn(ctx, token)
	}

	return *t.Token, nil
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
