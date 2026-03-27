package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
)

const (
	deepSearchCreateConversationPath = "api/deepsearch.v1.Service/CreateConversation"
	deepSearchGetConversationPath    = "api/deepsearch.v1.Service/GetConversation"
	deepSearchListConversationsPath  = "api/deepsearch.v1.Service/ListConversationSummaries"
	deepSearchLegacyPath             = ".api/deepsearch/v1"
)

type deepSearchRunOptions struct {
	Question     string
	Wait         bool
	PollInterval time.Duration
	Timeout      time.Duration
}

var deepSearchNumericIDPattern = regexp.MustCompile(`^\d+$`)

func init() {
	usage := `
'src deep-search' runs a Deep Search conversation using the stable Sourcegraph API endpoints.

Usage:

	src deep-search [options] <question>

	Examples:

		$ src deep-search "How does authentication work in this repository?"
		$ src deep-search -json "List the files involved in code ownership checks"
		$ src deep-search -wait=false "Find all references to SearchJobFields"
		$ src deep-search -read "users/~self/conversations/140"
		$ src deep-search -read "https://sourcegraph.example.com/deepsearch/shared/caebeb05-7755-4f89-834f-e3ee4a6acb25"
		$ src deep-search -list
`

	flagSet := flag.NewFlagSet("deep-search", flag.ExitOnError)
	apiFlags := api.NewFlags(flagSet)
	jsonFlag := flagSet.Bool("json", false, "Output the full conversation JSON response")
	readFlag := flagSet.String("read", "", "Read an existing conversation by name, numeric ID, or Deep Search URL/read token")
	listFlag := flagSet.Bool("list", false, "List Deep Search conversation summaries")
	limitFlag := flagSet.Int("limit", 20, "Maximum number of conversation summaries to request when -list is set")
	waitFlag := flagSet.Bool("wait", true, "Wait for Deep Search processing to finish")
	pollIntervalFlag := flagSet.Duration("poll-interval", 2*time.Second, "Polling interval when -wait is enabled")
	timeoutFlag := flagSet.Duration("timeout", 2*time.Minute, "Maximum time to wait when -wait is enabled")

	handler := func(args []string) error {
		if err := flagSet.Parse(args); err != nil {
			return err
		}

		if *pollIntervalFlag <= 0 {
			return cmderrors.Usage("-poll-interval must be greater than zero")
		}
		if *timeoutFlag <= 0 {
			return cmderrors.Usage("-timeout must be greater than zero")
		}
		if *limitFlag <= 0 {
			return cmderrors.Usage("-limit must be greater than zero")
		}
		if *readFlag != "" && *listFlag {
			return cmderrors.Usage("-read and -list cannot be used together")
		}

		client := cfg.apiClient(apiFlags, flagSet.Output())
		if *listFlag {
			if flagSet.NArg() != 0 {
				return cmderrors.Usage("do not pass a question when -list is set")
			}
			result, err := deepSearchListConversationSummaries(context.Background(), client, *limitFlag)
			if err != nil {
				return err
			}
			if *jsonFlag {
				formatted, err := marshalIndent(result)
				if err != nil {
					return err
				}
				fmt.Println(string(formatted))
				return nil
			}
			summaries, err := deepSearchExtractSummaries(result)
			if err != nil {
				return err
			}
			for _, summary := range summaries {
				name, _ := deepSearchStringField(summary, "name")
				title, _ := deepSearchStringField(summary, "title")
				updatedAt, _ := deepSearchStringField(summary, "updatedAt", "updated_at")
				fmt.Printf("%s\t%s\t%s\n", name, title, updatedAt)
			}
			return nil
		}

		var conversation map[string]any
		var err error
		if *readFlag != "" {
			if flagSet.NArg() != 0 {
				return cmderrors.Usage("do not pass a question when -read is set")
			}
			conversation, err = readDeepSearchConversation(context.Background(), client, *readFlag)
			if err != nil {
				return err
			}
		} else {
			if flagSet.NArg() == 0 {
				return cmderrors.Usage("must provide a Deep Search question")
			}
			conversation, err = runDeepSearch(context.Background(), client, deepSearchRunOptions{
				Question:     strings.Join(flagSet.Args(), " "),
				Wait:         *waitFlag,
				PollInterval: *pollIntervalFlag,
				Timeout:      *timeoutFlag,
			})
			if err != nil {
				return err
			}
		}

		if *jsonFlag {
			formatted, err := marshalIndent(conversation)
			if err != nil {
				return err
			}
			fmt.Println(string(formatted))
			return nil
		}

		question, err := deepSearchLatestQuestion(conversation)
		if err != nil {
			return err
		}

		if answer, ok := deepSearchLatestAnswerText(question); ok && answer != "" {
			fmt.Println(answer)
		} else {
			if name, ok := deepSearchStringField(conversation, "name"); ok && name != "" {
				fmt.Printf("Conversation: %s\n", name)
			}
			if state, ok := deepSearchConversationState(conversation, question); ok && state != "" {
				fmt.Printf("State: %s\n", state)
			}
		}

		if followups := deepSearchSuggestedFollowups(question); len(followups) > 0 {
			fmt.Println("\nSuggested follow-ups:")
			for _, followup := range followups {
				fmt.Printf("- %s\n", followup)
			}
		}

		return nil
	}

	commands = append(commands, &command{
		flagSet: flagSet,
		aliases: []string{"deepsearch", "ds"},
		handler: handler,
		usageFunc: func() {
			fmt.Fprintf(flag.CommandLine.Output(), "Usage of 'src %s':\n", flagSet.Name())
			flagSet.PrintDefaults()
			fmt.Println(usage)
		},
	})
}

func readDeepSearchConversation(ctx context.Context, client api.Client, identifier string) (map[string]any, error) {
	conversationName, readToken := parseDeepSearchIdentifier(identifier)
	if conversationName != "" {
		return deepSearchGetConversation(ctx, client, conversationName)
	}
	if readToken != "" {
		return deepSearchGetConversationByReadToken(ctx, client, readToken)
	}
	return nil, fmt.Errorf("could not parse deep search identifier %q", identifier)
}

func deepSearchListConversationSummaries(ctx context.Context, client api.Client, limit int) (map[string]any, error) {
	var result map[string]any
	payload := map[string]any{
		"parent": "users/~self",
	}
	if limit > 0 {
		payload["pageSize"] = limit
	}
	if err := deepSearchPostJSON(ctx, client, deepSearchListConversationsPath, payload, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func parseDeepSearchIdentifier(identifier string) (conversationName string, readToken string) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return "", ""
	}

	// Stable API resource names.
	if strings.HasPrefix(identifier, "users/") && strings.Contains(identifier, "/conversations/") {
		return identifier, ""
	}

	// Current-user numeric ID shorthand.
	if deepSearchNumericIDPattern.MatchString(identifier) {
		return "users/~self/conversations/" + identifier, ""
	}

	// Deep Search web URLs.
	if u, err := url.Parse(identifier); err == nil && u.Scheme != "" && u.Host != "" {
		segments := strings.Split(strings.Trim(path.Clean(u.Path), "/"), "/")
		if len(segments) >= 2 && segments[0] == "deepsearch" {
			// /deepsearch/<id_or_token>
			if len(segments) == 2 {
				if deepSearchNumericIDPattern.MatchString(segments[1]) {
					return "users/~self/conversations/" + segments[1], ""
				}
				return "", segments[1]
			}
			// /deepsearch/shared/<token>
			if len(segments) >= 3 && segments[1] == "shared" {
				return "", segments[2]
			}
		}
	}

	// Fallback: treat non-space string as token.
	if !strings.Contains(identifier, " ") {
		return "", identifier
	}

	return "", ""
}

func runDeepSearch(ctx context.Context, client api.Client, opts deepSearchRunOptions) (map[string]any, error) {
	conversation, err := deepSearchCreateConversation(ctx, client, opts.Question)
	if err != nil {
		return nil, err
	}
	if !opts.Wait {
		return conversation, nil
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	for {
		question, err := deepSearchLatestQuestion(conversation)
		if err != nil {
			return nil, err
		}
		state, _ := deepSearchConversationState(conversation, question)
		if !deepSearchIsProcessingState(state) {
			if deepSearchIsFailureState(state) {
				return nil, fmt.Errorf("deep search finished with state %q", state)
			}
			return conversation, nil
		}

		name, ok := deepSearchStringField(conversation, "name")
		if !ok || name == "" {
			return nil, fmt.Errorf("deep search response did not include conversation name")
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for deep search response: %w", ctx.Err())
		case <-time.After(opts.PollInterval):
		}

		conversation, err = deepSearchGetConversation(ctx, client, name)
		if err != nil {
			return nil, err
		}
	}
}

func deepSearchCreateConversation(ctx context.Context, client api.Client, question string) (map[string]any, error) {
	var conversation map[string]any
	if err := deepSearchPostJSON(ctx, client, deepSearchCreateConversationPath, map[string]any{
		"parent": "users/~self",
		"conversation": map[string]any{
			"questions": []map[string]any{
				{
					"input": []map[string]any{
						{
							"question": map[string]any{
								"text": question,
							},
						},
					},
				},
			},
		},
	}, &conversation); err != nil {
		return nil, err
	}
	return conversation, nil
}

func deepSearchGetConversation(ctx context.Context, client api.Client, name string) (map[string]any, error) {
	var conversation map[string]any
	if err := deepSearchPostJSON(ctx, client, deepSearchGetConversationPath, map[string]any{
		"name": name,
	}, &conversation); err != nil {
		return nil, err
	}
	return conversation, nil
}

func deepSearchGetConversationByReadToken(ctx context.Context, client api.Client, readToken string) (map[string]any, error) {
	req, err := client.NewHTTPRequest(ctx, http.MethodGet, deepSearchLegacyPath+"?filter_read_token="+url.QueryEscape(readToken), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error: %s\n\n%s", resp.Status, respBody)
	}

	return deepSearchExtractConversationFromLegacyList(respBody)
}

func deepSearchPostJSON(ctx context.Context, client api.Client, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := client.NewHTTPRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error: %s\n\n%s", resp.Status, respBody)
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return err
	}

	return nil
}

func deepSearchExtractConversationFromLegacyList(respBody []byte) (map[string]any, error) {
	var conversations []map[string]any
	if err := json.Unmarshal(respBody, &conversations); err == nil {
		if len(conversations) == 0 {
			return nil, fmt.Errorf("no deep search conversations found")
		}
		return conversations[0], nil
	}

	var wrapped map[string]any
	if err := json.Unmarshal(respBody, &wrapped); err != nil {
		return nil, err
	}

	if _, ok := wrapped["questions"]; ok {
		return wrapped, nil
	}

	for _, key := range []string{"conversations", "results", "items", "data"} {
		raw, ok := wrapped[key]
		if !ok {
			continue
		}
		list, ok := raw.([]any)
		if !ok || len(list) == 0 {
			continue
		}
		conversation, ok := list[0].(map[string]any)
		if ok {
			return conversation, nil
		}
	}

	return nil, fmt.Errorf("could not parse conversation from response")
}

func deepSearchExtractSummaries(response map[string]any) ([]map[string]any, error) {
	for _, key := range []string{"conversationSummaries", "summaries", "conversations", "results", "items", "data"} {
		raw, ok := response[key]
		if !ok {
			continue
		}
		list, ok := raw.([]any)
		if !ok {
			continue
		}
		summaries := make([]map[string]any, 0, len(list))
		for _, item := range list {
			summary, ok := item.(map[string]any)
			if ok {
				summaries = append(summaries, summary)
			}
		}
		if len(summaries) > 0 {
			return summaries, nil
		}
	}
	return nil, fmt.Errorf("deep search response did not include conversation summaries")
}

func deepSearchLatestQuestion(conversation map[string]any) (map[string]any, error) {
	questionsRaw, ok := conversation["questions"]
	if !ok {
		return nil, fmt.Errorf("deep search response did not include questions")
	}

	questions, ok := questionsRaw.([]any)
	if !ok || len(questions) == 0 {
		return nil, fmt.Errorf("deep search response did not include any questions")
	}

	question, ok := questions[len(questions)-1].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("deep search response contained malformed question entry")
	}

	return question, nil
}

func deepSearchQuestionState(question map[string]any) (string, bool) {
	if state, ok := deepSearchStringField(question, "state", "status"); ok {
		return state, true
	}
	return "", false
}

func deepSearchConversationState(conversation map[string]any, question map[string]any) (string, bool) {
	if stateValue, ok := conversation["state"]; ok {
		if stateString, ok := stateValue.(string); ok && stateString != "" {
			return stateString, true
		}
		if stateObj, ok := stateValue.(map[string]any); ok {
			switch {
			case stateObj["processing"] != nil:
				return "STATE_PROCESSING", true
			case stateObj["completed"] != nil:
				return "STATE_COMPLETED", true
			case stateObj["canceled"] != nil || stateObj["cancelled"] != nil:
				return "STATE_CANCELED", true
			case stateObj["error"] != nil:
				return "STATE_ERROR", true
			}
		}
	}

	// Legacy compatibility.
	return deepSearchQuestionState(question)
}

func deepSearchLatestAnswerText(question map[string]any) (string, bool) {
	// Legacy format.
	if answer, ok := deepSearchStringField(question, "answer"); ok && answer != "" {
		return answer, true
	}

	// Stable API format: answer is an array of blocks, usually markdown blocks.
	raw, ok := question["answer"]
	if !ok {
		return "", false
	}
	blocks, ok := raw.([]any)
	if !ok || len(blocks) == 0 {
		return "", false
	}

	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		blockMap, ok := block.(map[string]any)
		if !ok {
			continue
		}
		markdown, ok := blockMap["markdown"].(map[string]any)
		if !ok {
			continue
		}
		text, ok := markdown["text"].(string)
		if ok && text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "\n\n"), true
}

func deepSearchSuggestedFollowups(question map[string]any) []string {
	var raw any
	if v, ok := question["suggestedFollowups"]; ok {
		raw = v
	} else if v, ok := question["suggested_followups"]; ok {
		raw = v
	} else {
		return nil
	}

	list, ok := raw.([]any)
	if !ok {
		return nil
	}

	followups := make([]string, 0, len(list))
	for _, item := range list {
		if followup, ok := item.(string); ok && followup != "" {
			followups = append(followups, followup)
		}
	}
	return followups
}

func deepSearchStringField(m map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		str, ok := value.(string)
		if ok {
			return str, true
		}
	}
	return "", false
}

func deepSearchIsProcessingState(state string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(state))
	return normalized == "STATE_PROCESSING" || normalized == "PROCESSING"
}

func deepSearchIsFailureState(state string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(state))
	return strings.Contains(normalized, "FAILED") || strings.Contains(normalized, "ERROR") || strings.Contains(normalized, "CANCELLED") || strings.Contains(normalized, "CANCELED")
}
