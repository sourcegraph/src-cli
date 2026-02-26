package oauthdevice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockRoundTripper struct {
	handler func(*http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.handler(req)
}

func TestTransport_SetsAuthorizationHeader(t *testing.T) {
	var capturedAuth string

	transport := &Transport{
		Base: &mockRoundTripper{
			handler: func(req *http.Request) (*http.Response, error) {
				capturedAuth = req.Header.Get("Authorization")
				return &http.Response{StatusCode: 200}, nil
			},
		},
		Token: &Token{
			AccessToken: "test-token",
			ExpiresAt:   time.Now().Add(time.Hour),
		},
	}

	req := httptest.NewRequest("GET", "http://example.com", nil)
	_, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}

	if capturedAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", capturedAuth, "Bearer test-token")
	}
}

func TestMaybeRefresh_RefreshesExpiredToken(t *testing.T) {
	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testTokenPath: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh","expires_in":3600}`))
			},
		},
	})
	defer server.Close()

	token := &Token{
		Endpoint:     server.URL,
		AccessToken:  "expired-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(-time.Hour), // expired
	}

	result, err := maybeRefresh(context.Background(), token)
	if err != nil {
		t.Fatalf("maybeRefresh() error = %v", err)
	}

	if result.AccessToken != "new-token" {
		t.Errorf("AccessToken = %q, want %q", result.AccessToken, "new-token")
	}
}

func TestMaybeRefresh_RefreshesTokenExpiringSoon(t *testing.T) {
	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testTokenPath: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh","expires_in":3600}`))
			},
		},
	})
	defer server.Close()

	token := &Token{
		Endpoint:     server.URL,
		AccessToken:  "expiring-soon-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(10 * time.Second), // expires in 10s (< 30s threshold)
	}

	result, err := maybeRefresh(context.Background(), token)
	if err != nil {
		t.Fatalf("maybeRefresh() error = %v", err)
	}

	if result.AccessToken != "new-token" {
		t.Errorf("AccessToken = %q, want %q", result.AccessToken, "new-token")
	}
}

func TestTransport_RefreshPersistence(t *testing.T) {
	tests := []struct {
		name              string
		needsRefresh      bool
		persistErr        error
		wantAuthHeaderVal string
		wantStoreCalls    int
	}{
		{
			name:              "persists refreshed token",
			needsRefresh:      true,
			wantAuthHeaderVal: "Bearer new-token",
			wantStoreCalls:    1,
		},
		{
			name:              "does not persist unchanged token",
			wantAuthHeaderVal: "Bearer valid-token",
			wantStoreCalls:    0,
		},
		{
			name:              "persist failure does not fail request",
			needsRefresh:      true,
			persistErr:        errors.New("persist failed"),
			wantAuthHeaderVal: "Bearer new-token",
			wantStoreCalls:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalStoreFn := storeRefreshedTokenFn
			defer func() { storeRefreshedTokenFn = originalStoreFn }()

			var storeCalls int
			var storedToken *Token
			storeRefreshedTokenFn = func(_ context.Context, token *Token) error {
				storeCalls++
				storedToken = token
				return tt.persistErr
			}

			token := &Token{
				AccessToken: "valid-token",
				ExpiresAt:   time.Now().Add(time.Hour),
			}
			if tt.needsRefresh {
				server := newTestServer(t, testServerOptions{
					handlers: map[string]http.HandlerFunc{
						testTokenPath: func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Content-Type", "application/json")
							w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh","expires_in":3600}`))
						},
					},
				})
				defer server.Close()
				token.Endpoint = server.URL
				token.AccessToken = "expired-token"
				token.RefreshToken = "refresh-token"
				token.ExpiresAt = time.Now().Add(-time.Hour)
			}

			var capturedAuth string
			transport := &Transport{
				Base: &mockRoundTripper{
					handler: func(req *http.Request) (*http.Response, error) {
						capturedAuth = req.Header.Get("Authorization")
						return &http.Response{StatusCode: 200}, nil
					},
				},
				Token: token,
			}

			req := httptest.NewRequest("GET", "http://example.com", nil)
			_, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip() error = %v", err)
			}

			if capturedAuth != tt.wantAuthHeaderVal {
				t.Errorf("Authorization = %q, want %q", capturedAuth, tt.wantAuthHeaderVal)
			}
			if storeCalls != tt.wantStoreCalls {
				t.Errorf("store calls = %d, want %d", storeCalls, tt.wantStoreCalls)
			}

			if tt.needsRefresh {
				if storedToken == nil {
					t.Fatal("stored token is nil")
				}
				if storedToken.AccessToken != "new-token" {
					t.Errorf("stored AccessToken = %q, want %q", storedToken.AccessToken, "new-token")
				}
			}
		})
	}
}
