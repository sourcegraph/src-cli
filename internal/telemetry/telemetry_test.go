package telemetry

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"
	apimock "github.com/sourcegraph/src-cli/internal/api/mock"

	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func testSource() Source {
	return Source{Client: ClientName, ClientVersion: "6.1.0"}
}

func TestRecord_SendsWellFormedMutation(t *testing.T) {
	client := &apimock.Client{}
	req := &apimock.Request{}

	var gotQuery string
	var gotVars map[string]any
	client.On("NewRequest", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			gotQuery = args.Get(0).(string)
			gotVars = args.Get(1).(map[string]any)
		}).
		Return(req)
	req.On("Do", mock.Anything, mock.Anything).Return(true, nil)

	rec := NewRecorder(client, testSource())
	rec.Record(context.Background(), Event{
		Feature:  "srcCli.search",
		Action:   "succeeded",
		Metadata: map[string]float64{"durationMs": 12, "exitCode": 0},
	})

	assert.Equal(t, recordEventsMutation, gotQuery)

	events, ok := gotVars["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected 1 event, got %#v", gotVars["events"])
	}
	event := events[0].(map[string]any)
	assert.Equal(t, "srcCli.search", event["feature"])
	assert.Equal(t, "succeeded", event["action"])

	source := event["source"].(map[string]any)
	assert.Equal(t, ClientName, source["client"])
	assert.Equal(t, "6.1.0", source["clientVersion"])

	params := event["parameters"].(map[string]any)
	assert.Equal(t, eventParametersVersion, params["version"])

	metadata := params["metadata"].([]any)
	// sorted by key: durationMs, exitCode
	assert.Equal(t, []any{
		map[string]any{"key": "durationMs", "value": float64(12)},
		map[string]any{"key": "exitCode", "value": float64(0)},
	}, metadata)

	client.AssertExpectations(t)
	req.AssertExpectations(t)
}

func TestRecord_EmptyMetadataSendsEmptyList(t *testing.T) {
	client := &apimock.Client{}
	req := &apimock.Request{}

	var gotVars map[string]any
	client.On("NewRequest", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { gotVars = args.Get(1).(map[string]any) }).
		Return(req)
	req.On("Do", mock.Anything, mock.Anything).Return(true, nil)

	rec := NewRecorder(client, testSource())
	rec.Record(context.Background(), Event{Feature: "srcCli.version", Action: "succeeded"})

	event := gotVars["events"].([]any)[0].(map[string]any)
	params := event["parameters"].(map[string]any)
	assert.Equal(t, []any{}, params["metadata"])
}

func TestRecord_ValidationFailsBeforeSending(t *testing.T) {
	client := &apimock.Client{}
	// No expectations set: NewRequest must never be called.

	rec := NewRecorder(client, testSource())
	err := rec.record(context.Background(), Event{Feature: "Bad_Feature", Action: "succeeded"})

	assert.Error(t, err)
	client.AssertNotCalled(t, "NewRequest", mock.Anything, mock.Anything)
}

func TestRecord_NetworkErrorSwallowed(t *testing.T) {
	client := &apimock.Client{}
	req := &apimock.Request{}
	client.On("NewRequest", mock.Anything, mock.Anything).Return(req)
	req.On("Do", mock.Anything, mock.Anything).Return(false, errors.New("connection refused"))

	var debug bytes.Buffer
	rec := NewRecorder(client, testSource(), WithDebug(&debug))

	// Must not panic and must not surface the error.
	assert.NotPanics(t, func() {
		rec.Record(context.Background(), Event{Feature: "srcCli.search", Action: "failed"})
	})
	assert.Contains(t, debug.String(), "connection refused")

	// record itself reports the error for callers that want it.
	err := rec.record(context.Background(), Event{Feature: "srcCli.search", Action: "failed"})
	assert.Error(t, err)
}

func TestRecord_GraphQLErrorSwallowed(t *testing.T) {
	// Simulates an instance too old to have the telemetry mutation: the server
	// returns GraphQL errors, which must be dropped silently.
	client := &apimock.Client{}
	req := &apimock.Request{}
	client.On("NewRequest", mock.Anything, mock.Anything).Return(req)
	req.On("Do", mock.Anything, mock.Anything).
		Return(false, api.GraphQlErrors{})

	rec := NewRecorder(client, testSource())
	assert.NotPanics(t, func() {
		rec.Record(context.Background(), Event{Feature: "srcCli.search", Action: "succeeded"})
	})
}

func TestRecord_NilClientDoesNotPanic(t *testing.T) {
	rec := NewRecorder(nil, testSource())
	assert.NotPanics(t, func() {
		rec.Record(context.Background(), Event{Feature: "srcCli.search", Action: "succeeded"})
	})
}

func TestRecord_AppliesTimeout(t *testing.T) {
	client := &apimock.Client{}
	req := &apimock.Request{}

	var hadDeadline bool
	client.On("NewRequest", mock.Anything, mock.Anything).Return(req)
	req.On("Do", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			ctx := args.Get(0).(context.Context)
			_, hadDeadline = ctx.Deadline()
		}).
		Return(true, nil)

	rec := NewRecorder(client, testSource(), WithTimeout(50*time.Millisecond))
	rec.Record(context.Background(), Event{Feature: "srcCli.search", Action: "succeeded"})

	assert.True(t, hadDeadline, "expected Record to apply a context deadline")
}
