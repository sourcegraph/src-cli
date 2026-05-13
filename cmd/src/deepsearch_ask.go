package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/clicompat"
	"github.com/sourcegraph/src-cli/internal/cmderrors"
	"github.com/sourcegraph/src-cli/internal/deepsearch"
	"github.com/urfave/cli/v3"
)

const deepsearchAskExamples = `
Examples:

  Ask a question and wait for the answer:

    	$ src deepsearch ask 'How is authentication implemented?'

  Ask a question using the ds alias:

    	$ src ds ask 'Which services write repository metadata?'

`

var deepsearchAskCommand = clicompat.Wrap(&cli.Command{
	Name:        "ask",
	Usage:       "starts a Deep Search conversation and waits for the answer",
	UsageText:   "src deepsearch ask [options] <question>",
	Description: deepsearchAskExamples,
	HideVersion: true,
	Flags: clicompat.WithAPIFlags(
		&cli.StringFlag{
			Name:  "parent",
			Usage: `Parent resource for the conversation. Defaults to the authenticated user. (e.g. "users/-")`,
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Value: 5 * time.Minute,
			Usage: "Maximum time to wait for an answer.",
		},
		&cli.DurationFlag{
			Name:  "poll-interval",
			Value: 3 * time.Second,
			Usage: "How often to poll for completion.",
		},
	),
	Action: func(ctx context.Context, cmd *cli.Command) error {
		question, err := deepsearchQuestion(cmd)
		if err != nil {
			return err
		}
		timeout := cmd.Duration("timeout")
		if timeout <= 0 {
			return cmderrors.Usage("timeout must be greater than 0")
		}
		pollInterval := cmd.Duration("poll-interval")
		if pollInterval <= 0 {
			return cmderrors.Usage("poll-interval must be greater than 0")
		}

		client := cfg.deepsearchClient(cmd)
		conversation, ok, err := client.CreateConversation(ctx, deepsearch.CreateConversationRequest{
			Parent: cmd.String("parent"),
			Conversation: deepsearch.Conversation{
				Questions: []deepsearch.Question{deepsearch.NewQuestion(question)},
			},
		})
		if err != nil || !ok {
			return err
		}

		return waitForDeepsearchAnswer(ctx, cmd.Writer, client, conversation, timeout, pollInterval)
	},
})

func deepsearchQuestion(cmd *cli.Command) (string, error) {
	question := strings.TrimSpace(strings.Join(cmd.Args().Slice(), " "))
	if question == "" {
		return "", cmderrors.Usage("must provide a question")
	}
	return question, nil
}

func waitForDeepsearchAnswer(ctx context.Context, out io.Writer, client *deepsearch.Client, conversation *deepsearch.Conversation, timeout, pollInterval time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		done, err := deepsearchDone(conversation)
		if err != nil {
			return err
		}
		if done {
			return printDeepsearchAnswer(out, conversation)
		}
		if conversation.Name == "" {
			return errors.New("deep search response did not include a conversation name")
		}

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return errors.Newf("timed out waiting for Deep Search answer after %s", timeout)
			}
			return ctx.Err()
		case <-ticker.C:
		}

		var ok bool
		conversation, ok, err = client.GetConversation(ctx, deepsearch.GetConversationRequest{Name: conversation.Name})
		if err != nil || !ok {
			return err
		}
	}
}

func deepsearchDone(conversation *deepsearch.Conversation) (bool, error) {
	if conversation == nil || conversation.State == nil || conversation.State.Processing != nil {
		return false, nil
	}
	if conversation.State.Completed != nil {
		return true, nil
	}
	if conversation.State.Canceled != nil {
		return false, errors.New("Deep Search conversation was canceled")
	}
	if conversation.State.Error != nil {
		message := conversation.State.Error.Message
		if message == "" {
			message = conversation.State.Error.Code
		}
		if message == "" {
			message = "unknown error"
		}
		return false, errors.Newf("Deep Search failed: %s", message)
	}
	return false, nil
}

func printDeepsearchAnswer(out io.Writer, conversation *deepsearch.Conversation) error {
	for _, question := range conversation.Questions {
		for _, answer := range question.Answer {
			if answer.Markdown == nil {
				continue
			}
			if _, err := fmt.Fprint(out, answer.Markdown.Text); err != nil {
				return err
			}
			if !strings.HasSuffix(answer.Markdown.Text, "\n") {
				if _, err := fmt.Fprintln(out); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
