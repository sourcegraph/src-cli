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

	srv := &http.Server{
		Handler: &mockHandler{
			data: resp,
			t:    t,
		},
	}
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

// mockHandler implements a HTTP handler that always returns the given data as its response, and always succeeds.
type mockHandler struct {
	data []byte
	t    interface {
		Logf(format string, args ...interface{})
	}
}

var _ http.Handler = &mockHandler{}

func (ms *mockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ms.t != nil {
		ms.t.Logf("handling request: %+v", r)
	}

	w.WriteHeader(200)
	w.Write(ms.data)
}
