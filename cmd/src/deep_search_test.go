package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"
)

func TestRunDeepSearch_WaitsForCompletion(t *testing.T) {
	t.Parallel()

	var getCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/" + deepSearchCreateConversationPath:
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			if body["parent"] != "users/~self" {
				t.Fatalf("unexpected parent body: %v", body["parent"])
			}

			conversation, ok := body["conversation"].(map[string]any)
			if !ok {
				t.Fatalf("missing conversation body: %v", body["conversation"])
			}
			questions, ok := conversation["questions"].([]any)
			if !ok || len(questions) == 0 {
				t.Fatalf("missing questions body: %v", conversation["questions"])
			}
			q, ok := questions[0].(map[string]any)
			if !ok {
				t.Fatalf("malformed question body: %v", questions[0])
			}
			input, ok := q["input"].([]any)
			if !ok || len(input) == 0 {
				t.Fatalf("missing input body: %v", q["input"])
			}
			block, ok := input[0].(map[string]any)
			if !ok {
				t.Fatalf("malformed input block: %v", input[0])
			}
			question, ok := block["question"].(map[string]any)
			if !ok || question["text"] != "Does this repo have a README?" {
				t.Fatalf("unexpected question block: %v", block["question"])
			}
			_, _ = io.WriteString(w, `{"name":"users/~self/conversations/140","state":{"processing":{}},"questions":[{}]}`)
		case "/" + deepSearchGetConversationPath:
			call := atomic.AddInt32(&getCalls, 1)
			if call == 1 {
				_, _ = io.WriteString(w, `{"name":"users/~self/conversations/140","state":{"processing":{}},"questions":[{}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"name":"users/~self/conversations/140","state":{"completed":{}},"questions":[{"answer":[{"markdown":{"text":"Yes"}}]}]}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL)

	conversation, err := runDeepSearch(context.Background(), client, deepSearchRunOptions{
		Question:     "Does this repo have a README?",
		Wait:         true,
		PollInterval: 1 * time.Millisecond,
		Timeout:      1 * time.Second,
	})
	if err != nil {
		t.Fatalf("runDeepSearch returned error: %v", err)
	}

	if name, _ := deepSearchStringField(conversation, "name"); name != "users/~self/conversations/140" {
		t.Fatalf("unexpected conversation name: %q", name)
	}

	question, err := deepSearchLatestQuestion(conversation)
	if err != nil {
		t.Fatalf("deepSearchLatestQuestion returned error: %v", err)
	}

	if state, _ := deepSearchConversationState(conversation, question); state != "STATE_COMPLETED" {
		t.Fatalf("unexpected state: %q", state)
	}
	if answer, _ := deepSearchLatestAnswerText(question); answer != "Yes" {
		t.Fatalf("unexpected answer: %q", answer)
	}
}

func TestRunDeepSearch_ReturnsFailureState(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"users/~self/conversations/140","questions":[{"state":"STATE_FAILED"}]}`)
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL)

	_, err := runDeepSearch(context.Background(), client, deepSearchRunOptions{
		Question:     "Does this fail?",
		Wait:         true,
		PollInterval: 1 * time.Millisecond,
		Timeout:      1 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "STATE_FAILED") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeepSearchSuggestedFollowups(t *testing.T) {
	t.Parallel()

	camel := deepSearchSuggestedFollowups(map[string]any{"suggestedFollowups": []any{"one", "two"}})
	if len(camel) != 2 || camel[0] != "one" || camel[1] != "two" {
		t.Fatalf("unexpected camel case followups: %v", camel)
	}

	snake := deepSearchSuggestedFollowups(map[string]any{"suggested_followups": []any{"a"}})
	if len(snake) != 1 || snake[0] != "a" {
		t.Fatalf("unexpected snake case followups: %v", snake)
	}
}

func TestDeepSearchConversationState(t *testing.T) {
	t.Parallel()

	state, ok := deepSearchConversationState(map[string]any{"state": map[string]any{"processing": map[string]any{}}}, map[string]any{})
	if !ok || state != "STATE_PROCESSING" {
		t.Fatalf("unexpected processing state: (%q, %v)", state, ok)
	}

	state, ok = deepSearchConversationState(map[string]any{"state": map[string]any{"completed": map[string]any{}}}, map[string]any{})
	if !ok || state != "STATE_COMPLETED" {
		t.Fatalf("unexpected completed state: (%q, %v)", state, ok)
	}

	// Legacy fallback.
	state, ok = deepSearchConversationState(map[string]any{}, map[string]any{"status": "completed"})
	if !ok || state != "completed" {
		t.Fatalf("unexpected legacy fallback state: (%q, %v)", state, ok)
	}
}

func TestDeepSearchLatestAnswerText(t *testing.T) {
	t.Parallel()

	answer, ok := deepSearchLatestAnswerText(map[string]any{"answer": "legacy"})
	if !ok || answer != "legacy" {
		t.Fatalf("unexpected legacy answer: (%q, %v)", answer, ok)
	}

	answer, ok = deepSearchLatestAnswerText(map[string]any{
		"answer": []any{
			map[string]any{"markdown": map[string]any{"text": "first"}},
			map[string]any{"markdown": map[string]any{"text": "second"}},
		},
	})
	if !ok || answer != "first\n\nsecond" {
		t.Fatalf("unexpected block answer: (%q, %v)", answer, ok)
	}
}

func TestParseDeepSearchIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		wantName string
		wantRead string
	}{
		{
			input:    "users/~self/conversations/140",
			wantName: "users/~self/conversations/140",
		},
		{
			input:    "140",
			wantName: "users/~self/conversations/140",
		},
		{
			input:    "https://sourcegraph.example.com/deepsearch/140",
			wantName: "users/~self/conversations/140",
		},
		{
			input:    "https://sourcegraph.example.com/deepsearch/caebeb05-7755-4f89-834f-e3ee4a6acb25",
			wantRead: "caebeb05-7755-4f89-834f-e3ee4a6acb25",
		},
		{
			input:    "https://sourcegraph.example.com/deepsearch/shared/caebeb05-7755-4f89-834f-e3ee4a6acb25",
			wantRead: "caebeb05-7755-4f89-834f-e3ee4a6acb25",
		},
		{
			input:    "caebeb05-7755-4f89-834f-e3ee4a6acb25",
			wantRead: "caebeb05-7755-4f89-834f-e3ee4a6acb25",
		},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			name, read := parseDeepSearchIdentifier(tc.input)
			if name != tc.wantName || read != tc.wantRead {
				t.Fatalf("parseDeepSearchIdentifier(%q) = (%q, %q), want (%q, %q)", tc.input, name, read, tc.wantName, tc.wantRead)
			}
		})
	}
}

func TestReadDeepSearchConversation_ReadTokenFallback(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/"+deepSearchLegacyPath {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("filter_read_token"); got != "caebeb05-7755-4f89-834f-e3ee4a6acb25" {
			t.Fatalf("unexpected read token query: %q", got)
		}
		_, _ = io.WriteString(w, `{"conversations":[{"name":"users/abc/conversations/140","questions":[{"state":"STATE_COMPLETED","answer":"answer"}]}]}`)
	}))
	defer server.Close()

	client := newTestAPIClient(t, server.URL)

	conversation, err := readDeepSearchConversation(context.Background(), client, "https://sourcegraph.example.com/deepsearch/shared/caebeb05-7755-4f89-834f-e3ee4a6acb25")
	if err != nil {
		t.Fatalf("readDeepSearchConversation returned error: %v", err)
	}
	if name, _ := deepSearchStringField(conversation, "name"); name != "users/abc/conversations/140" {
		t.Fatalf("unexpected conversation name: %q", name)
	}
}

func TestDeepSearchExtractSummaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     map[string]any
		wantCount int
		wantErr   bool
	}{
		{
			name: "conversationSummaries",
			input: map[string]any{
				"conversationSummaries": []any{
					map[string]any{"name": "users/~self/conversations/1"},
					map[string]any{"name": "users/~self/conversations/2"},
				},
			},
			wantCount: 2,
		},
		{
			name: "results",
			input: map[string]any{
				"results": []any{
					map[string]any{"name": "users/~self/conversations/1"},
				},
			},
			wantCount: 1,
		},
		{
			name:    "missing",
			input:   map[string]any{"foo": "bar"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			summaries, err := deepSearchExtractSummaries(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(summaries); got != tc.wantCount {
				t.Fatalf("unexpected summary count: got %d, want %d", got, tc.wantCount)
			}
		})
	}
}

func newTestAPIClient(t *testing.T, endpoint string) api.Client {
	t.Helper()

	flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
	apiFlags := api.NewFlags(flagSet)

	return api.NewClient(api.ClientOpts{
		Endpoint:    endpoint,
		AccessToken: "test-token",
		Out:         io.Discard,
		Flags:       apiFlags,
	})
}
