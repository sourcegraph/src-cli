package oauthdevice

import (
	"context"
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
