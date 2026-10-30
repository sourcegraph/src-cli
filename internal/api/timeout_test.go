package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func newTestClient(t *testing.T, serverURL string) Client {
	t.Helper()
	endpointURL, err := url.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	return NewClient(ClientOpts{EndpointURL: endpointURL, Out: io.Discard})
}

func TestUnresponsiveServerTimesOut(t *testing.T) {
	t.Setenv("SRC_RESPONSE_HEADER_TIMEOUT", "100ms")

	block := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer server.Close()
	defer close(block)

	client := newTestClient(t, server.URL)
	req, err := client.NewHTTPRequest(context.Background(), http.MethodGet, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected a timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("request took %s, expected it to fail within the response header timeout", elapsed)
	}
}

func TestSlowStreamingDownloadSucceeds(t *testing.T) {
	t.Setenv("SRC_RESPONSE_HEADER_TIMEOUT", "200ms")

	const chunks = 5
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		// Stream the body for much longer than the response header timeout.
		for range chunks {
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte("chunk"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	req, err := client.NewHTTPRequest(context.Background(), http.MethodGet, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading a slow streaming body failed: %v", err)
	}
	if want := len("chunk") * chunks; len(body) != want {
		t.Fatalf("got %d body bytes, want %d", len(body), want)
	}
}

func TestResponseHeaderTimeoutEnv(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{value: "", want: defaultResponseHeaderTimeout},
		{value: "30s", want: 30 * time.Second},
		{value: "0", want: 0},
		{value: "garbage", want: defaultResponseHeaderTimeout},
		{value: "-5s", want: defaultResponseHeaderTimeout},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("SRC_RESPONSE_HEADER_TIMEOUT", test.value)
			if got := responseHeaderTimeout(); got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
		})
	}
}
