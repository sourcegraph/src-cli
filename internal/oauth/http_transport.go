package oauthdevice

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

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	token, err := maybeRefresh(ctx, t.Token)
	if err != nil {
		return nil, err
	}
	t.Token = token

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
	client := NewClient(DefaultClientID)

	resp, err := client.Refresh(ctx, token)
	if err != nil {
		return nil, err
	}

	return resp.Token(token.Endpoint), nil
}
