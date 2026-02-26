package oauth

import (
	"context"
	"net/http"
	"time"
)

var _ http.Transport

var _ http.RoundTripper = (*Transport)(nil)

type Transport struct {
	Base  http.RoundTripper
	Token *Token
}

// storeRefreshedTokenFn is the function the transport should use to persist the token - mainly used during
// tests to swap out the implementation out with a mock
var storeRefreshedTokenFn = StoreToken

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	prevToken := t.Token
	token, err := maybeRefresh(ctx, t.Token)
	if err != nil {
		return nil, err
	}
	t.Token = token
	if token != prevToken {
		// try to save the token if we fail let the request continue with in memory token
		_ = storeRefreshedTokenFn(ctx, token)
	}

	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+t.Token.AccessToken)

	if t.Base != nil {
		return t.Base.RoundTrip(req2)
	}
	return http.DefaultTransport.RoundTrip(req2)
}

func maybeRefresh(ctx context.Context, token *Token) (*Token, error) {
	// token has NOT expired or NOT about to expire in 30s
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
