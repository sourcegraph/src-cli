package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testDeviceAuthPath = "/device/code"
	testTokenPath      = "/token"
)

type testServerOptions struct {
	handlers      map[string]http.HandlerFunc
	wellKnownFunc func(w http.ResponseWriter, r *http.Request)
}

func newTestServer(t *testing.T, opts testServerOptions) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case wellKnownPath:
			if opts.wellKnownFunc != nil {
				opts.wellKnownFunc(w, r)
			} else {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(OIDCConfiguration{
					Issuer:                      "http://" + r.Host,
					DeviceAuthorizationEndpoint: "http://" + r.Host + testDeviceAuthPath,
					TokenEndpoint:               "http://" + r.Host + testTokenPath,
				})
			}
		default:
			if handler, ok := opts.handlers[r.URL.Path]; ok {
				handler(w, r)
			} else {
				t.Errorf("unexpected path: %s", r.URL.Path)
				http.Error(w, "not found", http.StatusNotFound)
			}
		}
	}))
}

func TestDiscover_Success(t *testing.T) {
	server := newTestServer(t, testServerOptions{})
	defer server.Close()

	client := NewClient(DefaultClientID)
	config, err := client.Discover(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if config.DeviceAuthorizationEndpoint != server.URL+testDeviceAuthPath {
		t.Errorf("DeviceAuthorizationEndpoint = %q, want %q", config.DeviceAuthorizationEndpoint, server.URL+testDeviceAuthPath)
	}
	if config.TokenEndpoint != server.URL+testTokenPath {
		t.Errorf("TokenEndpoint = %q, want %q", config.TokenEndpoint, server.URL+testTokenPath)
	}
}

func TestDiscover_Caching(t *testing.T) {
	var callCount int32
	server := newTestServer(t, testServerOptions{
		wellKnownFunc: func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&callCount, 1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(OIDCConfiguration{
				DeviceAuthorizationEndpoint: "http://example.com/device",
				TokenEndpoint:               "http://example.com/token",
			})
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID)

	// Populate the cache
	_, err := client.Discover(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Second call should use cache
	_, err = client.Discover(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("callCount = %d, want 1 (second call should use cache)", callCount)
	}
}

func TestDiscover_Error(t *testing.T) {
	server := newTestServer(t, testServerOptions{
		wellKnownFunc: func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID)
	_, err := client.Discover(context.Background(), server.URL)
	if err == nil {
		t.Fatal("Discover() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want to contain '404'", err.Error())
	}
}

func TestStart_Success(t *testing.T) {
	wantResponse := DeviceAuthResponse{
		DeviceCode:              "test-device-code",
		UserCode:                "ABCD-1234",
		VerificationURI:         "https://example.com/device",
		VerificationURIComplete: "https://example.com/device?user_code=ABCD-1234",
		ExpiresIn:               1800,
		Interval:                5,
	}

	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testDeviceAuthPath: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("unexpected method: %s", r.Method)
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				if err := r.ParseForm(); err != nil {
					t.Errorf("failed to parse form: %v", err)
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}

				if got := r.FormValue("client_id"); got != DefaultClientID {
					t.Errorf("unexpected client_id: got %q, want %q", got, DefaultClientID)
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(wantResponse)
			},
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID)
	resp, err := client.Start(context.Background(), server.URL, nil)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if resp.DeviceCode != wantResponse.DeviceCode {
		t.Errorf("DeviceCode = %q, want %q", resp.DeviceCode, wantResponse.DeviceCode)
	}
	if resp.UserCode != wantResponse.UserCode {
		t.Errorf("UserCode = %q, want %q", resp.UserCode, wantResponse.UserCode)
	}
	if resp.VerificationURI != wantResponse.VerificationURI {
		t.Errorf("VerificationURI = %q, want %q", resp.VerificationURI, wantResponse.VerificationURI)
	}
	if resp.VerificationURIComplete != wantResponse.VerificationURIComplete {
		t.Errorf("VerificationURIComplete = %q, want %q", resp.VerificationURIComplete, wantResponse.VerificationURIComplete)
	}
	if resp.ExpiresIn != wantResponse.ExpiresIn {
		t.Errorf("ExpiresIn = %d, want %d", resp.ExpiresIn, wantResponse.ExpiresIn)
	}
	if resp.Interval != wantResponse.Interval {
		t.Errorf("Interval = %d, want %d", resp.Interval, wantResponse.Interval)
	}
}

func TestStart_WithScopes(t *testing.T) {
	var receivedScope string

	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testDeviceAuthPath: func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Errorf("failed to parse form: %v", err)
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				receivedScope = r.FormValue("scope")

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(DeviceAuthResponse{
					DeviceCode:      "test-device-code",
					UserCode:        "ABCD-1234",
					VerificationURI: "https://example.com/device",
					ExpiresIn:       1800,
					Interval:        5,
				})
			},
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID)
	_, err := client.Start(context.Background(), server.URL, []string{"read", "write"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if receivedScope != "read write" {
		t.Errorf("scope = %q, want %q", receivedScope, "read write")
	}
}

func TestStart_Error(t *testing.T) {
	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testDeviceAuthPath: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(ErrorResponse{
					Error:            "invalid_client",
					ErrorDescription: "Unknown client",
				})
			},
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID)
	_, err := client.Start(context.Background(), server.URL, nil)
	if err == nil {
		t.Fatal("Start() expected error, got nil")
	}

	wantErr := "device auth failed: invalid_client: Unknown client"
	if err.Error() != wantErr {
		t.Errorf("error = %q, want %q", err.Error(), wantErr)
	}
}

func TestStart_NoDeviceEndpoint(t *testing.T) {
	server := newTestServer(t, testServerOptions{
		wellKnownFunc: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(OIDCConfiguration{
				TokenEndpoint: "http://example.com/token",
			})
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID)
	_, err := client.Start(context.Background(), server.URL, nil)
	if err == nil {
		t.Fatal("Start() expected error, got nil")
	}

	if !strings.Contains(err.Error(), "device authorization endpoint not found") {
		t.Errorf("error = %q, want to contain 'device authorization endpoint not found'", err.Error())
	}
}

func TestPoll_Success(t *testing.T) {
	wantToken := TokenResponse{
		AccessToken: "test-access-token",
		ExpiresIn:   3600,
		Scope:       "read write",
		TokenType:   "Bearer",
	}

	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testTokenPath: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("unexpected method: %s", r.Method)
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}

				if err := r.ParseForm(); err != nil {
					t.Errorf("failed to parse form: %v", err)
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}

				if got := r.FormValue("client_id"); got != DefaultClientID {
					t.Errorf("unexpected client_id: got %q, want %q", got, DefaultClientID)
				}
				if got := r.FormValue("grant_type"); got != GrantTypeDeviceCode {
					t.Errorf("unexpected grant_type: got %q", got)
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(wantToken)
			},
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID).(*httpClient)
	resp, err := client.Poll(context.Background(), server.URL, "test-device-code", 10*time.Millisecond, 60)
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	if resp.AccessToken != wantToken.AccessToken {
		t.Errorf("AccessToken = %q, want %q", resp.AccessToken, wantToken.AccessToken)
	}
	if resp.TokenType != wantToken.TokenType {
		t.Errorf("TokenType = %q, want %q", resp.TokenType, wantToken.TokenType)
	}

}

func TestPoll_AuthorizationPending(t *testing.T) {
	var callCount int32

	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testTokenPath: func(w http.ResponseWriter, r *http.Request) {
				count := atomic.AddInt32(&callCount, 1)

				w.Header().Set("Content-Type", "application/json")

				if count < 3 {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(ErrorResponse{
						Error:            "authorization_pending",
						ErrorDescription: "The user has not yet completed authorization",
					})
					return
				}

				json.NewEncoder(w).Encode(TokenResponse{
					AccessToken: "test-access-token",
					TokenType:   "Bearer",
				})
			},
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID).(*httpClient)
	resp, err := client.Poll(context.Background(), server.URL, "test-device-code", 10*time.Millisecond, 60)
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	if resp.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %q, want %q", resp.AccessToken, "test-access-token")
	}

	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
}

func TestPoll_SlowDown(t *testing.T) {
	var callCount int32

	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testTokenPath: func(w http.ResponseWriter, r *http.Request) {
				count := atomic.AddInt32(&callCount, 1)

				w.Header().Set("Content-Type", "application/json")

				if count == 1 {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(ErrorResponse{
						Error: "slow_down",
					})
					return
				}

				json.NewEncoder(w).Encode(TokenResponse{
					AccessToken: "test-access-token",
					TokenType:   "Bearer",
				})
			},
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID).(*httpClient)
	resp, err := client.Poll(context.Background(), server.URL, "test-device-code", 10*time.Millisecond, 60)
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}

	if resp.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %q, want %q", resp.AccessToken, "test-access-token")
	}

	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("callCount = %d, want 2", callCount)
	}
}

func TestPoll_ExpiredToken(t *testing.T) {
	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testTokenPath: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(ErrorResponse{
					Error:            "expired_token",
					ErrorDescription: "The device code has expired",
				})
			},
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID).(*httpClient)
	_, err := client.Poll(context.Background(), server.URL, "test-device-code", 10*time.Millisecond, 60)
	if err == nil {
		t.Fatal("Poll() expected error, got nil")
	}

	wantErr := "device code expired"
	if err.Error() != wantErr {
		t.Errorf("error = %q, want %q", err.Error(), wantErr)
	}
}

func TestPoll_AccessDenied(t *testing.T) {
	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testTokenPath: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(ErrorResponse{
					Error:            "access_denied",
					ErrorDescription: "The user denied the request",
				})
			},
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID).(*httpClient)
	_, err := client.Poll(context.Background(), server.URL, "test-device-code", 10*time.Millisecond, 60)
	if err == nil {
		t.Fatal("Poll() expected error, got nil")
	}

	wantErr := "authorization was denied by the user"
	if err.Error() != wantErr {
		t.Errorf("error = %q, want %q", err.Error(), wantErr)
	}
}

func TestPoll_Timeout(t *testing.T) {
	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testTokenPath: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(ErrorResponse{
					Error: "authorization_pending",
				})
			},
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID).(*httpClient)
	_, err := client.Poll(context.Background(), server.URL, "test-device-code", 10*time.Millisecond, 0)
	if err == nil {
		t.Fatal("Poll() expected error, got nil")
	}

	wantErr := "device code expired"
	if err.Error() != wantErr {
		t.Errorf("error = %q, want %q", err.Error(), wantErr)
	}
}

func TestPoll_ContextCancellation(t *testing.T) {
	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testTokenPath: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(ErrorResponse{
					Error: "authorization_pending",
				})
			},
		},
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewClient(DefaultClientID).(*httpClient)
	_, err := client.Poll(ctx, server.URL, "test-device-code", 10*time.Millisecond, 3600)
	if err == nil {
		t.Fatal("Poll() expected error, got nil")
	}

	if err != context.Canceled && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("error = %v, want context.Canceled or wrapped context canceled error", err)
	}
}

func TestRefresh_Success(t *testing.T) {
	server := newTestServer(t, testServerOptions{
		handlers: map[string]http.HandlerFunc{
			testTokenPath: func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				if got := r.FormValue("grant_type"); got != "refresh_token" {
					t.Errorf("grant_type = %q, want %q", got, "refresh_token")
				}
				if got := r.FormValue("refresh_token"); got != "test-refresh-token" {
					t.Errorf("refresh_token = %q, want %q", got, "test-refresh-token")
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(TokenResponse{
					AccessToken:  "new-access-token",
					RefreshToken: "new-refresh-token",
					ExpiresIn:    3600,
					TokenType:    "Bearer",
				})
			},
		},
	})
	defer server.Close()

	client := NewClient(DefaultClientID)
	token := &Token{
		Endpoint:     server.URL,
		AccessToken:  "new-access-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Add(time.Second * time.Duration(3600)),
	}
	resp, err := client.Refresh(context.Background(), token)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	if resp.AccessToken != "new-access-token" {
		t.Errorf("AccessToken = %q, want %q", resp.AccessToken, "new-access-token")
	}
	if resp.RefreshToken != "new-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", resp.RefreshToken, "new-refresh-token")
	}
}

func TestRefresh_DiscoverFailure(t *testing.T) {
	client := NewClient(DefaultClientID)
	token := &Token{
		Endpoint:     "http://127.0.0.1:1",
		RefreshToken: "test-refresh-token",
	}

	_, err := client.Refresh(context.Background(), token)
	if err == nil {
		t.Fatal("Refresh() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to discover OIDC configuration") {
		t.Errorf("error = %q, want discovery failure context", err.Error())
	}
}
