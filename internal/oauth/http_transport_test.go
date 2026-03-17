package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newRefreshServer(t *testing.T, accessToken string) *httptest.Server {
	t.Helper()
	return newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testTokenPath: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"` + accessToken + `","refresh_token":"new-refresh","expires_in":3600}`))
			},
		},
	})
}

func TestTokenRefresherGetToken(t *testing.T) {
	server := newRefreshServer(t, "new-token")
	defer server.Close()

	originalStoreFn := storeRefreshedTokenFn
	storeRefreshedTokenFn = func(context.Context, *Token) error { return nil }
	defer func() { storeRefreshedTokenFn = originalStoreFn }()

	tests := []struct {
		name       string
		token      *Token
		wantAccess string
		wantSame   bool
	}{
		{
			name: "unchanged when still valid",
			token: &Token{
				AccessToken: "valid-token",
				ExpiresAt:   time.Now().Add(time.Hour),
			},
			wantAccess: "valid-token",
			wantSame:   true,
		},
		{
			name: "refreshes expired token",
			token: &Token{
				Endpoint:     server.URL,
				AccessToken:  "expired-token",
				RefreshToken: "refresh-token",
				ExpiresAt:    time.Now().Add(-time.Hour),
			},
			wantAccess: "new-token",
		},
		{
			name: "refreshes token expiring soon",
			token: &Token{
				Endpoint:     server.URL,
				AccessToken:  "expiring-soon-token",
				RefreshToken: "refresh-token",
				ExpiresAt:    time.Now().Add(10 * time.Second),
			},
			wantAccess: "new-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refresher := NewTokenRefresher(tt.token)
			got, err := refresher.GetToken(context.Background())
			if err != nil {
				t.Fatalf("GetToken() error = %v", err)
			}
			if got.AccessToken != tt.wantAccess {
				t.Errorf("AccessToken = %q, want %q", got.AccessToken, tt.wantAccess)
			}
			if tt.wantSame && refresher.token != tt.token {
				t.Errorf("token pointer changed for unexpired token")
			}
		})
	}
}

func TestTransportRoundTrip(t *testing.T) {
	tests := []struct {
		name           string
		token          *Token
		persistErr     error
		wantAuthHeader string
		wantStoreCalls int
	}{
		{
			name: "uses existing token without persisting",
			token: &Token{
				AccessToken: "valid-token",
				ExpiresAt:   time.Now().Add(time.Hour),
			},
			wantAuthHeader: "Bearer valid-token",
			wantStoreCalls: 0,
		},
		{
			name: "persists refreshed token",
			token: &Token{
				AccessToken:  "expired-token",
				RefreshToken: "refresh-token",
				ExpiresAt:    time.Now().Add(-time.Hour),
			},
			wantAuthHeader: "Bearer new-token",
			wantStoreCalls: 1,
		},
		{
			name: "ignores persist failures",
			token: &Token{
				AccessToken:  "expired-token",
				RefreshToken: "refresh-token",
				ExpiresAt:    time.Now().Add(-time.Hour),
			},
			persistErr:     errors.New("persist failed"),
			wantAuthHeader: "Bearer new-token",
			wantStoreCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantStoreCalls > 0 {
				server := newRefreshServer(t, "new-token")
				defer server.Close()
				tt.token.Endpoint = server.URL
			}

			originalStoreFn := storeRefreshedTokenFn
			defer func() { storeRefreshedTokenFn = originalStoreFn }()

			var storeCalls int
			var storedToken *Token
			storeRefreshedTokenFn = func(_ context.Context, token *Token) error {
				storeCalls++
				storedToken = token
				return tt.persistErr
			}

			var capturedAuth string
			tr := NewTransport(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				capturedAuth = req.Header.Get("Authorization")
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
			}), tt.token)

			_, err := tr.RoundTrip(httptest.NewRequest(http.MethodGet, "http://example.com", nil))
			if err != nil {
				t.Fatalf("RoundTrip() error = %v", err)
			}

			if capturedAuth != tt.wantAuthHeader {
				t.Errorf("Authorization = %q, want %q", capturedAuth, tt.wantAuthHeader)
			}
			if storeCalls != tt.wantStoreCalls {
				t.Errorf("store calls = %d, want %d", storeCalls, tt.wantStoreCalls)
			}
			if tt.wantStoreCalls > 0 && (storedToken == nil || storedToken.AccessToken != "new-token") {
				t.Errorf("stored token = %#v, want access token %q", storedToken, "new-token")
			}
		})
	}
}
