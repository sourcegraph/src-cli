package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"
	apimock "github.com/sourcegraph/src-cli/internal/api/mock"
	"github.com/sourcegraph/src-cli/internal/oauth"

	"github.com/sourcegraph/log"
	"github.com/sourcegraph/log/logtest"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const testClientVersion = "6.1.0"

func response(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type cancellationClient struct {
	api.Client
	canceled chan struct{}
	release  chan struct{}
}

func (c *cancellationClient) NewHTTPRequest(ctx context.Context, method, _ string, body io.Reader) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, "http://example.com/.api/graphql", body)
}

func (c *cancellationClient) Do(req *http.Request) (*http.Response, error) {
	select {
	case <-req.Context().Done():
		close(c.canceled)
		return nil, req.Context().Err()
	case <-c.release:
		return nil, errors.New("test client released")
	}
}

func TestRecord_SendsWellFormedMutation(t *testing.T) {
	client := &apimock.Client{}
	req := httptest.NewRequest(http.MethodPost, "/.api/graphql", nil)

	var gotPayload struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/graphql", mock.Anything).
		Run(func(args mock.Arguments) {
			if err := json.NewDecoder(args.Get(3).(io.Reader)).Decode(&gotPayload); err != nil {
				t.Fatal(err)
			}
		}).
		Return(req, nil)
	client.On("Do", req).Return(response(http.StatusOK, "{}"), nil)

	logger := log.NoOp()
	rec := NewRecorder(client, logger, testClientVersion)
	rec.Record(context.Background(), "srcCli.search", "succeeded", map[string]float64{
		"durationMs": 12,
		"exitCode":   0,
	}, map[string]any{
		"queryType": "literal",
		"streamed":  true,
	})

	assert.Equal(t, recordEventsMutation, gotPayload.Query)

	events, ok := gotPayload.Variables["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected 1 event, got %#v", gotPayload.Variables["events"])
	}
	event := events[0].(map[string]any)
	assert.Equal(t, "srcCli.search", event["feature"])
	assert.Equal(t, "succeeded", event["action"])

	source := event["source"].(map[string]any)
	assert.Equal(t, clientName, source["client"])
	assert.Equal(t, testClientVersion, source["clientVersion"])

	parameters := event["parameters"].(map[string]any)
	assert.Equal(t, float64(eventParametersVersion), parameters["version"])
	assert.Equal(t, []any{
		map[string]any{"key": "durationMs", "value": float64(12)},
		map[string]any{"key": "exitCode", "value": float64(0)},
	}, parameters["metadata"])
	assert.Equal(t, map[string]any{
		"queryType": "literal",
		"streamed":  true,
	}, parameters["privateMetadata"])

	client.AssertExpectations(t)
}

func TestRecord_NilMetadataSendsEmptyList(t *testing.T) {
	client := &apimock.Client{}
	req := httptest.NewRequest(http.MethodPost, "/.api/graphql", nil)

	var gotPayload struct {
		Variables map[string]any `json:"variables"`
	}
	client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/graphql", mock.Anything).
		Run(func(args mock.Arguments) {
			if err := json.NewDecoder(args.Get(3).(io.Reader)).Decode(&gotPayload); err != nil {
				t.Fatal(err)
			}
		}).
		Return(req, nil)
	client.On("Do", req).Return(response(http.StatusOK, "{}"), nil)

	logger := log.NoOp()
	rec := NewRecorder(client, logger, testClientVersion)
	rec.Record(context.Background(), "srcCli.version", "succeeded", nil, nil)

	event := gotPayload.Variables["events"].([]any)[0].(map[string]any)
	parameters := event["parameters"].(map[string]any)
	assert.Equal(t, float64(eventParametersVersion), parameters["version"])
	assert.Equal(t, []any{}, parameters["metadata"])
	assert.NotContains(t, parameters, "privateMetadata")
}

func TestRecord_NetworkErrorSwallowed(t *testing.T) {
	client := &apimock.Client{}
	req := httptest.NewRequest(http.MethodPost, "/.api/graphql", nil)
	client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/graphql", mock.Anything).Return(req, nil)
	client.On("Do", req).Return(nil, errors.New("connection refused"))

	logger, exportLogs := logtest.CapturedWith(t, logtest.LoggerOptions{Level: log.LevelNone})
	rec := NewRecorder(client, logger, testClientVersion)

	// Must not panic and must not surface the error.
	assert.NotPanics(t, func() {
		rec.Record(context.Background(), "srcCli.search", "failed", nil, nil)
	})
	logs := exportLogs()
	if assert.Len(t, logs, 1) {
		assert.Equal(t, log.LevelDebug, logs[0].Level)
		assert.Equal(t, "recording telemetry event", logs[0].Message)
		assert.Equal(t, "connection refused", logs[0].Fields["error"])
	}

	// record itself reports the error for callers that want it.
	err := rec.record(context.Background(), "srcCli.search", "failed", nil, nil)
	assert.Error(t, err)
}

func TestRecord_GraphQLErrorSwallowed(t *testing.T) {
	// Simulates an instance too old to have the telemetry mutation: the server
	// returns GraphQL errors, which must be dropped silently.
	client := &apimock.Client{}
	req := httptest.NewRequest(http.MethodPost, "/.api/graphql", nil)
	client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/graphql", mock.Anything).Return(req, nil)
	client.On("Do", req).Return(response(http.StatusOK, "{\"errors\":[{\"message\":\"unknown field telemetry\"}]}"), nil)

	logger := log.NoOp()
	rec := NewRecorder(client, logger, testClientVersion)
	assert.NotPanics(t, func() {
		rec.Record(context.Background(), "srcCli.search", "succeeded", nil, nil)
	})
}

func TestRecord_OAuthUnauthorizedDoesNotWriteToStdout(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, "Bearer oauth-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	endpointURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	var clientOutput bytes.Buffer
	client := api.NewClient(api.ClientOpts{
		EndpointURL: endpointURL,
		Out:         &clientOutput,
		OAuthToken: &oauth.Token{
			Endpoint:    server.URL,
			AccessToken: "oauth-token",
			ExpiresAt:   time.Now().Add(time.Hour),
		},
	})

	oldStdout := os.Stdout
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = stdoutWriter
	t.Cleanup(func() { os.Stdout = oldStdout })

	logger := log.NoOp()
	rec := NewRecorder(client, logger, testClientVersion)
	rec.Record(context.Background(), "srcCli.search", "succeeded", nil, nil)

	if err := stdoutWriter.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = oldStdout
	stdout, err := io.ReadAll(stdoutReader)
	if err != nil {
		t.Fatal(err)
	}
	if err := stdoutReader.Close(); err != nil {
		t.Fatal(err)
	}
	assert.Empty(t, stdout)
	assert.Empty(t, clientOutput.String())
	assert.Equal(t, int32(1), requests.Load())
}

func TestRecord_AppliesTimeout(t *testing.T) {
	client := &apimock.Client{}
	req := httptest.NewRequest(http.MethodPost, "/.api/graphql", nil)

	var hadDeadline bool
	client.On("NewHTTPRequest", mock.Anything, http.MethodPost, ".api/graphql", mock.Anything).
		Run(func(args mock.Arguments) {
			ctx := args.Get(0).(context.Context)
			_, hadDeadline = ctx.Deadline()
		}).
		Return(req, nil)
	client.On("Do", req).Return(response(http.StatusOK, "{}"), nil)

	logger := log.NoOp()
	rec := NewRecorder(client, logger, testClientVersion)
	rec.timeout = 50 * time.Millisecond
	rec.Record(context.Background(), "srcCli.search", "succeeded", nil, nil)

	assert.True(t, hadDeadline, "expected Record to apply a context deadline")
}

func TestRecord_TimeoutCancelsHTTPRequest(t *testing.T) {
	requestCanceled := make(chan struct{})
	client := &cancellationClient{canceled: requestCanceled, release: make(chan struct{})}
	logger := log.NoOp()
	rec := NewRecorder(client, logger, testClientVersion)
	rec.timeout = 20 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		rec.Record(ctx, "srcCli.search", "succeeded", nil, nil)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		cancel()
		close(client.release)
		<-done
		t.Fatal("Record did not return after telemetry timeout")
	}

	select {
	case <-requestCanceled:
	default:
		t.Fatal("HTTP request was not canceled after telemetry timeout")
	}
}
