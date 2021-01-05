// Package mock provides mocking capabilities for api.Client instances.
package mock

import (
	"bytes"
	"net"
	"net/http"
	"testing"

	"github.com/pkg/errors"

	"github.com/sourcegraph/src-cli/internal/api"
)

// ParrotClient creates a new API client that always receives the given response.
func ParrotClient(t *testing.T, resp []byte) (api.Client, error) {
	// We're going to implement this by standing up a HTTP server on a random
	// port that always returns the given response.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, errors.Wrap(err, "listening on random port")
	}

	srv := &http.Server{Handler: mockHandler(t, resp)}
	go srv.Serve(l)

	var buf bytes.Buffer
	t.Cleanup(func() {
		srv.Close()
		t.Logf("output from API client: %s", buf.String())
	})

	return api.NewClient(api.ClientOpts{
		Endpoint: "http://" + l.Addr().String(),
		Out:      &buf,
	}), nil
}

// mockHandler returns a HTTP handler that always returns the given data as its response, and always succeeds.
func mockHandler(t *testing.T, data []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("handling request: %+v", r)
		w.WriteHeader(200)
		w.Write(data)
	})
}
