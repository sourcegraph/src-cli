package main

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sourcegraph/src-cli/internal/campaigns"
	"github.com/sourcegraph/src-cli/internal/output"
)

func TestCampaignProgressPrinterIntegration(t *testing.T) {
	buf := &ttyBuf{}

	out := output.NewOutput(buf, output.OutputOpts{
		ForceTTY:   true,
		ForceColor: true,
		Verbose:    true,
	})

	printer := newCampaignProgressPrinter(out, true, 4)
	printer.forceNoSpinner = true

	printer.PrintStatuses([]*campaigns.TaskStatus{
		{
			RepoName:           "github.com/sourcegraph/sourcegraph",
			StartedAt:          time.Now(),
			CurrentlyExecuting: "echo Hello World > README.md",
		},
		{
			RepoName:           "github.com/sourcegraph/src-cli",
			StartedAt:          time.Now().Add(time.Duration(-5) * time.Second),
			CurrentlyExecuting: "Downloading archive",
		},
	})

	have := buf.Lines()
	want := []string{
		"⠋  Executing... (0/2, 0 errored)  ",
		"│                                                                                                                     ",
		"├── github.com/sourcegraph/sourcegraph  echo Hello World > README.md                                                0s",
		"└── github.com/sourcegraph/src-cli      Downloading archive                                                         0s",
	}

	if !cmp.Equal(want, have) {
		t.Fatalf("wrong output:\n%s", cmp.Diff(want, have))
	}

}

type ttyBuf struct {
	lines [][]byte

	line   int
	column int
}

func (t *ttyBuf) Write(b []byte) (int, error) {
	var cur int

	for cur < len(b) {
		switch b[cur] {
		case '\n':
			t.line++
			t.column = 0
		case '\x1b':
			// First of all: forgive me.
			//
			// Now. Looks like we ran into a VT100 escape code.
			// They follow this structure:
			//
			//      \x1b [ <digit> <command>
			//
			// So we jump over the \x1b[ and try to parse the digit.

			cur = cur + 2 // cur == '\x1b', cur + 1 == '['

			digitStart := cur
			for isDigit(b[cur]) {
				cur++
			}

			rawDigit := string(b[digitStart:cur])
			digit, err := strconv.ParseInt(rawDigit, 0, 64)
			if err != nil {
				return 0, err
			}

			command := b[cur]

			// Debug helper:
			// fmt.Printf("command=%q, digit=%d (t.line=%d, t.column=%d)\n", command, digit, t.line, t.column)

			switch command {
			case 'K':
				// reset current line
				if len(t.lines) > t.line {
					t.lines[t.line] = []byte{}
					t.column = 0
				}
			case 'A':
				// move line up by <digit>
				t.line = t.line - int(digit)

			case 'D':
				// *d*elete cursor by <digit> amount
				t.column = t.column - int(digit)
				if t.column < 0 {
					t.column = 0
				}

			case 'm':
				// noop

			case ';':
				// color, skip over until end of color command
				for b[cur] != 'm' {
					cur++
				}
			}

		default:
			t.writeToCurrentLine(b[cur])
		}

		cur++
	}

	return len(b), nil
}

func (t *ttyBuf) writeToCurrentLine(b byte) {
	if len(t.lines) == t.line {
		t.lines = append(t.lines, []byte{})
	}

	if len(t.lines[t.line]) <= t.column {
		t.lines[t.line] = append(t.lines[t.line], b)
	} else {
		t.lines[t.line][t.column] = b
	}
	t.column++
}

func (t *ttyBuf) Lines() []string {
	var lines []string
	for _, l := range t.lines {
		lines = append(lines, string(l))
	}
	return lines
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}
