package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/src-cli/internal/oauth"
)

func TestCredentialsAreNotSentOnCrossHostRedirect(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		value     string
		configure func(*ClientOpts, string)
	}{
		{
			name:   "OAuth token",
			header: "Authorization",
			value:  "Bearer oauth-secret-token",
			configure: func(opts *ClientOpts, endpoint string) {
				opts.OAuthToken = &oauth.Token{
					Endpoint:    endpoint,
					AccessToken: "oauth-secret-token",
					ExpiresAt:   time.Now().Add(time.Hour),
				}
			},
		},
		{
			name:   "custom auth-proxy header",
			header: "X-Dbx-Auth-Token",
			value:  "proxy-secret-token",
			configure: func(opts *ClientOpts, _ string) {
				opts.AdditionalHeaders = map[string]string{
					"X-Dbx-Auth-Token": "proxy-secret-token",
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var redirectTargetRequests atomic.Int32
			redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirectTargetRequests.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer redirectTarget.Close()

			var initialHeader string
			redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				initialHeader = r.Header.Get(test.header)
				http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
			}))
			defer redirector.Close()

			endpointURL, err := url.Parse(redirector.URL)
			if err != nil {
				t.Fatal(err)
			}
			opts := ClientOpts{EndpointURL: endpointURL, Out: io.Discard}
			test.configure(&opts, redirector.URL)
			client := NewClient(opts)

			req, err := client.NewHTTPRequest(context.Background(), http.MethodGet, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusFound {
				t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusFound)
			}
			if initialHeader != test.value {
				t.Fatalf("initial request %s header = %q, want %q", test.header, initialHeader, test.value)
			}
			if got := redirectTargetRequests.Load(); got != 0 {
				t.Fatalf("cross-host redirect target received %d requests, want 0", got)
			}
		})
	}
}

func TestClientFollowsSameHostRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/target", http.StatusFound)
	})
	mux.HandleFunc("/target", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	endpointURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(ClientOpts{EndpointURL: endpointURL, Out: io.Discard})

	req, err := client.NewHTTPRequest(context.Background(), http.MethodGet, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("got status %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}
