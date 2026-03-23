package oauth

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

var _ http.Transport

var _ http.RoundTripper = (*Transport)(nil)

const defaultRefreshWindow = 5 * time.Minute

type Transport struct {
	Base      http.RoundTripper
	refresher *TokenRefresher
}

type TokenRefresher struct {
	token *Token
	mu    sync.Mutex
}

func NewTokenRefresher(token *Token) *TokenRefresher {
	return &TokenRefresher{token: token}
}

func NewTransport(base http.RoundTripper, token *Token) *Transport {
	return &Transport{Base: base, refresher: NewTokenRefresher(token)}
}

// storeRefreshedTokenFn is the function the transport should use to persist the token - mainly used during
// tests to swap out the implementation out with a mock
var storeRefreshedTokenFn = StoreToken

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	token, err := t.refresher.GetToken(ctx)
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

// GetToken returns a value copy of the token. If the token has expired or expiring soon it will be refreshed before returning.
// Once the token is refreshed, the in-memory token is updated and a best effort is made to store the token.
//
// If storing the token fails, no error is returned. An error is only returned if refreshing the token
// fails.
func (r *TokenRefresher) GetToken(ctx context.Context) (Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	prevToken := r.token
	token, err := maybeRefreshToken(ctx, r.token)
	if err != nil {
		return Token{}, err
	}
	r.token = token
	if token != prevToken {
		// try to save the token if we fail let the request continue with in memory token
		_ = storeRefreshedTokenFn(ctx, token)
	}

	return *r.token, nil
}

// maybeRefreshToken conditionally refreshes the token. If the token has expired or is
// expiring within the default refresh window, it will be refreshed and the updated token returned.
// Otherwise, no refresh occurs and the original token is returned.
func maybeRefreshToken(ctx context.Context, token *Token) (*Token, error) {
	if token == nil {
		return nil, errors.New("token is nil")
	}

	// token has NOT expired and is NOT about to expire in 30s
	if !(token.HasExpired() || token.ExpiringIn(defaultRefreshWindow)) {
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
