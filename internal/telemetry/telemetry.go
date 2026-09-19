// Package telemetry records src-cli usage events to the Sourcegraph instance
// the CLI is authenticated to, using the Telemetry V2 `recordEvents` GraphQL
// mutation.
//
// Recording is strictly best-effort: a failure to record — whether a network
// error, a GraphQL error, a timeout, or an instance too old to support the
// mutation — must never affect the command the user actually ran. See
// .context/TELEMETRY.md for the design and event schema.
package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const (
	// clientName identifies src-cli as the source of telemetry events.
	clientName = "SRC_CLI"

	// eventParametersVersion is the schema version of the metadata we attach to
	// each event. Bump it when the shape of the metadata changes.
	eventParametersVersion = 1

	// defaultTimeout bounds how long a single Record call may spend recording.
	// It is deliberately short: telemetry is sent synchronously right before the
	// process exits, so it must not add meaningful latency.
	defaultTimeout = 2 * time.Second
)

// recordEventsMutation mirrors the mutation used by Sourcegraph's own clients.
// The `telemetry` mutation only exists on Sourcegraph 5.2+, so on older
// instances this returns GraphQL errors, which Record silently drops.
const recordEventsMutation = `mutation RecordTelemetryEvents($events: [TelemetryEventInput!]!) {
    telemetry {
        recordEvents(events: $events) {
            alwaysNil
        }
    }
}`

// Recorder records events through an api.Client.
type Recorder struct {
	client        api.Client
	clientVersion string
	timeout       time.Duration
}

// NewRecorder returns a Recorder for the given src-cli version.
func NewRecorder(client api.Client, clientVersion string) *Recorder {
	return &Recorder{
		client:        client,
		clientVersion: clientVersion,
		timeout:       defaultTimeout,
	}
}

// Record sends an event on a best-effort basis. Feature and action identify the
// event (for example, "srcCli.search" and "succeeded"). Metadata must contain
// only numeric, PII-free facts. Record never returns an error or panics: network,
// GraphQL, timeout, and old-instance failures are silently dropped. It applies
// its own timeout, so the caller's context need not carry a deadline.
func (r *Recorder) Record(ctx context.Context, feature, action string, metadata map[string]float64) {
	_ = r.record(ctx, feature, action, metadata)
}

// record does the work behind Record and returns any error, so it can be tested
// directly. Callers outside tests should use Record.
func (r *Recorder) record(ctx context.Context, feature, action string, metadata map[string]float64) error {
	if r.client == nil {
		return errors.New("nil api client")
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	vars := map[string]any{
		"events": []any{buildEventInput(r.clientVersion, feature, action, metadata)},
	}

	payload, err := json.Marshal(map[string]any{
		"query":     recordEventsMutation,
		"variables": vars,
	})
	if err != nil {
		return err
	}
	// Use the HTTP-level client because GraphQL Request.Do may print interactive
	// authentication guidance; telemetry must never affect command output.
	req, err := r.client.NewHTTPRequest(ctx, http.MethodPost, ".api/graphql", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Newf("telemetry request failed: %s", resp.Status)
	}
	var result struct {
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		return api.NewGraphQlErrors(result.Errors)
	}
	return nil
}

// buildEventInput builds a single TelemetryEventInput as a JSON-serializable map.
func buildEventInput(clientVersion, feature, action string, metadata map[string]float64) map[string]any {
	return map[string]any{
		"feature": feature,
		"action":  action,
		"source": map[string]any{
			"client":        clientName,
			"clientVersion": clientVersion,
		},
		"parameters": map[string]any{
			"version":  eventParametersVersion,
			"metadata": buildMetadata(metadata),
		},
	}
}

// buildMetadata converts numeric metadata into the list of {key, value} inputs
// the API expects, sorted by key for deterministic output.
func buildMetadata(metadata map[string]float64) []any {
	out := make([]any, 0, len(metadata))
	if len(metadata) == 0 {
		return out
	}
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, map[string]any{"key": k, "value": metadata[k]})
	}
	return out
}
