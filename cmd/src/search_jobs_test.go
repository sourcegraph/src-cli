package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sourcegraph/src-cli/internal/api"
)

func TestFetchSearchJobFile(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("results data"))
	})
	mux.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "something went wrong", http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	endpointURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg = &config{
		endpointURL: endpointURL,
		accessToken: "test-token",
	}
	defer func() { cfg = nil }()

	client := api.NewClient(api.ClientOpts{EndpointURL: endpointURL, Out: io.Discard})

	t.Run("success", func(t *testing.T) {
		body, err := fetchSearchJobFile(client, server.URL+"/ok")
		if err != nil {
			t.Fatal(err)
		}
		defer body.Close()

		data, err := io.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "results data" {
			t.Fatalf("got body %q, want %q", data, "results data")
		}
		if gotAuth != "token test-token" {
			t.Fatalf("got Authorization header %q, want %q", gotAuth, "token test-token")
		}
	})

	t.Run("non-200 response", func(t *testing.T) {
		_, err := fetchSearchJobFile(client, server.URL+"/fail")
		if err == nil {
			t.Fatal("expected an error for a non-200 response, got nil")
		}
		if !strings.Contains(err.Error(), "something went wrong") {
			t.Fatalf("error %q does not contain the response body", err)
		}
	})
}
