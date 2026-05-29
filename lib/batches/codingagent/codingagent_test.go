package codingagent_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kballard/go-shellquote"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/batches/codingagent"
	"github.com/sourcegraph/sourcegraph/lib/batches/template"
)

func TestRenderRunCommand_unknownType(t *testing.T) {
	agentStep := &batcheslib.CodingAgentStep{Type: "nope", Prompt: "x"}
	_, err := codingagent.RenderRunCommand(agentStep, "https://example/", &template.StepContext{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, codingagent.ErrUnknownType) {
		t.Fatalf("expected ErrUnknownType, got %v", err)
	}
}

func TestRenderRunCommand_promptShellQuoting(t *testing.T) {
	const repoName = `github.com/sourcegraph/sourcegraph`
	prompt := "You're working in the ${{ repository.name }} repository.\n" +
		"Add a README section describing the project; don't touch existing files."
	agentStep := &batcheslib.CodingAgentStep{
		Type:   batcheslib.CodingAgentTypeCodex,
		Prompt: prompt,
	}
	stepCtx := &template.StepContext{Repository: template.Repository{Name: repoName}}
	cmd, err := codingagent.RenderRunCommand(agentStep, "https://example/", stepCtx)
	if err != nil {
		t.Fatal(err)
	}

	// The rendered command appends the shell-quoted prompt as its final
	// argument. Round-tripping via shellquote.Split is unreliable here
	// because the install script contains POSIX shell comments with
	// apostrophes (e.g. "can't"), which shellquote does not understand
	// and would treat as unterminated quoted strings. Assert on the
	// suffix instead.
	wantPrompt := "You're working in the " + repoName + " repository.\n" +
		"Add a README section describing the project; don't touch existing files."
	wantQuoted := shellquote.Join(wantPrompt)
	if !strings.HasSuffix(cmd, wantQuoted) {
		tail := cmd
		if n := len(wantQuoted) + 50; len(cmd) > n {
			tail = cmd[len(cmd)-n:]
		}
		t.Fatalf("rendered cmd does not end with shell-quoted prompt:\n  want suffix: %q\n  cmd tail:    %q", wantQuoted, tail)
	}
}
