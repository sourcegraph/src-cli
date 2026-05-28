package codingagent_test

import (
	"errors"
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

	tokens, err := shellquote.Split(cmd)
	if err != nil {
		t.Fatal(err)
	}
	wantPrompt := "You're working in the " + repoName + " repository.\n" +
		"Add a README section describing the project; don't touch existing files."
	if got := tokens[len(tokens)-1]; got != wantPrompt {
		t.Fatalf("prompt mismatch:\n  got:  %q\n  want: %q", got, wantPrompt)
	}
}
