package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sourcegraph/src-cli/internal/api"
)

func TestCallSendsConnectJSONRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Method, http.MethodPost; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got, want := r.URL.Path, "/api/deepsearch.v1.Service/CreateConversation"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Connect-Protocol-Version"), "1"; got != want {
			t.Fatalf("Connect-Protocol-Version = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Content-Type"), "application/json"; got != want {
			t.Fatalf("Content-Type = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "token secret"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("X-Test"), "yes"; got != want {
			t.Fatalf("X-Test = %q, want %q", got, want)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if got, want := payload["question"], "how does search work?"; got != want {
			t.Fatalf("question = %q, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-trace", "trace-id")
		_, _ = io.WriteString(w, `{"name":"users/-/conversations/1"}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	client := newTestClient(t, server.URL, api.NewFlagsFromValues(false, false, true, false, false), &out)

	var response struct {
		Name string `json:"name"`
	}
	ok, err := client.NewCall("/deepsearch.v1.Service/CreateConversation", map[string]string{
		"question": "how does search work?",
	}).Do(context.Background(), &response)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got, want := response.Name, "users/-/conversations/1"; got != want {
		t.Fatalf("response name = %q, want %q", got, want)
	}
	if got, want := out.String(), "x-trace: trace-id\n"; got != want {
		t.Fatalf("trace output = %q, want %q", got, want)
	}
}

func TestCallGetCurlDoesNotSendRequest(t *testing.T) {
	var called atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called.Store(true)
	}))
	defer server.Close()

	var out bytes.Buffer
	client := newTestClient(t, server.URL, api.NewFlagsFromValues(false, true, false, false, false), &out)

	ok, err := client.NewCall("/deepsearch.v1.Service/GetConversation", map[string]string{
		"name": "users/-/conversations/1",
	}).Do(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
	if called.Load() {
		t.Fatal("server received request despite get-curl")
	}
	output := out.String()
	for _, want := range []string{
		"curl",
		"Authorization: token secret",
		"Connect-Protocol-Version: 1",
		"/api/deepsearch.v1.Service/GetConversation",
		"users/-/conversations/1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("curl output %q does not contain %q", output, want)
		}
	}
}

func TestCallConnectError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":"invalid_argument","message":"question is required"}`)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, api.NewFlagsFromValues(false, false, false, false, false), io.Discard)

	ok, err := client.NewCall("/deepsearch.v1.Service/CreateConversation", map[string]string{}).Do(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if ok {
		t.Fatal("ok = true, want false")
	}

	var connectErr *Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("err = %v (%T), want *connect.Error", err, err)
	}
	if got, want := connectErr.Code, "invalid_argument"; got != want {
		t.Fatalf("Code = %q, want %q", got, want)
	}
	if got, want := connectErr.Message, "question is required"; got != want {
		t.Fatalf("Message = %q, want %q", got, want)
	}
	if got, want := err.Error(), "error: 400 Bad Request\n\ninvalid_argument: question is required"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func newTestClient(t *testing.T, endpoint string, flags *api.Flags, out io.Writer) Client {
	t.Helper()
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	apiClient := api.NewClient(api.ClientOpts{
		EndpointURL:       endpointURL,
		AccessToken:       "secret",
		AdditionalHeaders: map[string]string{"X-Test": "yes"},
		Flags:             flags,
		Out:               out,
	})
	return NewClient(apiClient)
}
