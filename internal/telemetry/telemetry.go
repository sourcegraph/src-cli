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
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const (
	// ClientName identifies src-cli as the source of telemetry events.
	ClientName = "SRC_CLI"

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

// Source identifies the client emitting events. It is constant for the lifetime
// of a process.
type Source struct {
	// Client is the source client name, e.g. ClientName.
	Client string
	// ClientVersion is the src-cli version, e.g. "6.1.0" or "dev".
	ClientVersion string
}

// Event is a single telemetry event.
//
// Feature and Action carry the event's identity and are always exported by
// Sourcegraph, so command identity lives here (e.g. Feature "srcCli.search",
// Action "succeeded"). Metadata values are numeric-only and are also always
// exported; they must never contain user content. See .context/TELEMETRY.md.
type Event struct {
	// Feature is a noun describing what the event is about, e.g. "srcCli.search".
	Feature string
	// Action is a verb describing what happened, e.g. "succeeded" or "failed".
	Action string
	// Metadata holds numeric-only, PII-free facts about the event.
	Metadata map[string]float64
}

// Recorder records events for a single Source through an api.Client.
type Recorder struct {
	client  api.Client
	source  Source
	timeout time.Duration
	debug   io.Writer
}

// Option customizes a Recorder.
type Option func(*Recorder)

// WithTimeout overrides the default per-Record timeout.
func WithTimeout(d time.Duration) Option {
	return func(r *Recorder) {
		if d > 0 {
			r.timeout = d
		}
	}
}

// WithDebug sets a writer that receives a diagnostic line whenever an event is
// dropped. Intended to be wired to verbose (-v) output; leave unset for silence.
func WithDebug(w io.Writer) Option {
	return func(r *Recorder) { r.debug = w }
}

// NewRecorder returns a Recorder that records events for source through client.
func NewRecorder(client api.Client, source Source, opts ...Option) *Recorder {
	r := &Recorder{
		client:  client,
		source:  source,
		timeout: defaultTimeout,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Record sends event on a best-effort basis. It never returns an error and
// never panics: validation, network, GraphQL, timeout, and old-instance
// failures are all silently dropped (written to the debug writer if one was set
// via WithDebug). It applies its own timeout, so the caller's context need not
// carry a deadline.
func (r *Recorder) Record(ctx context.Context, event Event) {
	if err := r.record(ctx, event); err != nil && r.debug != nil {
		fmt.Fprintf(r.debug, "telemetry: dropping event %q/%q: %v\n", event.Feature, event.Action, err)
	}
}

// record does the work behind Record and returns any error, so it can be tested
// directly. Callers outside tests should use Record.
func (r *Recorder) record(ctx context.Context, event Event) error {
	if r.client == nil {
		return errors.New("nil api client")
	}
	if err := Validate(event.Feature, event.Action); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	vars := map[string]any{
		"events": []any{buildEventInput(r.source, event)},
	}

	// The recordEvents payload has no fields we care about; we only need to
	// know whether the request succeeded.
	var result struct {
		Telemetry struct {
			RecordEvents struct {
				AlwaysNil *string
			}
		}
	}
	if _, err := r.client.NewRequest(recordEventsMutation, vars).Do(ctx, &result); err != nil {
		return err
	}
	return nil
}

// buildEventInput builds a single TelemetryEventInput as a JSON-serializable map.
func buildEventInput(source Source, event Event) map[string]any {
	return map[string]any{
		"feature": event.Feature,
		"action":  event.Action,
		"source": map[string]any{
			"client":        source.Client,
			"clientVersion": source.ClientVersion,
		},
		"parameters": map[string]any{
			"version":  eventParametersVersion,
			"metadata": buildMetadata(event.Metadata),
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
