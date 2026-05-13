package main

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
	"testing"

	"github.com/urfave/cli/v3"
)

func TestDeepsearchCommandHelpUsesPositionalOperands(t *testing.T) {
	tests := []struct {
		name        string
		command     *cli.Command
		wantUsage   string
		wantNotHave []string
	}{
		{
			name:        "add question",
			command:     deepsearchAddQuestionCommand,
			wantUsage:   "src deepsearch add-question [options] <conversation-name> <question>",
			wantNotHave: []string{"--parent string", "--question string"},
		},
		{
			name:        "get",
			command:     deepsearchGetCommand,
			wantUsage:   "src deepsearch get [options] <conversation-name>",
			wantNotHave: []string{"--name string"},
		},
		{
			name:        "cancel",
			command:     deepsearchCancelCommand,
			wantUsage:   "src deepsearch cancel [options] <conversation-name>",
			wantNotHave: []string{"--name string"},
		},
		{
			name:        "delete",
			command:     deepsearchDeleteCommand,
			wantUsage:   "src deepsearch delete [options] <conversation-name>",
			wantNotHave: []string{"--name string"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			help := deepsearchCommandHelp(t, tt.command)
			if !strings.Contains(help, "USAGE:\n   "+tt.wantUsage) {
				t.Fatalf("help did not contain usage %q:\n%s", tt.wantUsage, help)
			}
			for _, notHave := range tt.wantNotHave {
				if strings.Contains(help, notHave) {
					t.Fatalf("help contained %q:\n%s", notHave, help)
				}
			}
		})
	}
}

func deepsearchCommandHelp(t *testing.T, command *cli.Command) string {
	t.Helper()

	var out bytes.Buffer
	cmd := *command
	cmd.Writer = &out
	cmd.ErrWriter = &out

	if err := cmd.Run(context.Background(), []string{cmd.Name, "-h"}); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestDeepsearchAskCommandPollsUntilCompleted(t *testing.T) {
	var getCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/deepsearch.v1.Service/CreateConversation":
			var payload struct {
				Conversation struct {
					Questions []struct {
						Input []struct {
							Question struct {
								Text string `json:"text"`
							} `json:"question"`
						} `json:"input"`
					} `json:"questions"`
				} `json:"conversation"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			got := payload.Conversation.Questions[0].Input[0].Question.Text
			if want := "How is search implemented?"; got != want {
				t.Fatalf("question = %q, want %q", got, want)
			}
			_, _ = w.Write([]byte(`{
				"name":"users/-/conversations/1",
				"state":{"processing":{}},
				"url":"https://example.com/deepsearch/1"
			}`))
		case "/api/deepsearch.v1.Service/GetConversation":
			getCalls++
			_, _ = w.Write([]byte(`{
				"name":"users/-/conversations/1",
				"state":{"completed":{}},
				"questions":[{"answer":[{"markdown":{"text":"Deep Search uses search and code intelligence."}}]}]
			}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	endpointURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	previousCfg := cfg
	cfg = &config{endpointURL: endpointURL, accessToken: "token"}
	defer func() { cfg = previousCfg }()

	var out bytes.Buffer
	cmd := *deepsearchAskCommand
	cmd.Writer = &out
	cmd.ErrWriter = &out

	err = cmd.Run(context.Background(), []string{"ask", "How is search implemented?", "--poll-interval", "1ms", "--timeout", "1s"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "Deep Search uses search and code intelligence.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if getCalls != 1 {
		t.Fatalf("GetConversation calls = %d, want 1", getCalls)
	}
}

func TestDeepsearchAddQuestionCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/deepsearch.v1.Service/AddConversationQuestion"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		var payload struct {
			Parent   string `json:"parent"`
			Question struct {
				Input []struct {
					Question struct {
						Text string `json:"text"`
					} `json:"question"`
				} `json:"input"`
			} `json:"question"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if got, want := payload.Parent, "users/-/conversations/abc123"; got != want {
			t.Fatalf("parent = %q, want %q", got, want)
		}
		gotQuestion := payload.Question.Input[0].Question.Text
		if want := "What calls this code?"; gotQuestion != want {
			t.Fatalf("question = %q, want %q", gotQuestion, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"input":[{"question":{"text":"What calls this code?"}}]}`))
	}))
	defer server.Close()

	endpointURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	previousCfg := cfg
	cfg = &config{endpointURL: endpointURL, accessToken: "token"}
	defer func() { cfg = previousCfg }()

	var cmdOut bytes.Buffer
	cmd := *deepsearchAddQuestionCommand
	cmd.Writer = &cmdOut
	cmd.ErrWriter = &cmdOut

	stdout := captureStdout(t, func() error {
		return cmd.Run(context.Background(), []string{
			"add-question",
			"users/-/conversations/abc123",
			"What calls this code?",
			"-f", "{{range .Input}}{{.Question.Text}}{{end}}",
		})
	})
	if got, want := stdout, "What calls this code?\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func captureStdout(t *testing.T, run func() error) string {
	t.Helper()
	previousStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = previousStdout }()

	if err := run(); err != nil {
		_ = writer.Close()
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}
