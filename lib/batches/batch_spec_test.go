package batches

import (
	"fmt"
	"strings"
	"testing"
)

// v3SpecWith wraps stepsYAML in the minimum scaffold needed for ParseBatchSpec
// to accept a v3 spec.
func v3SpecWith(stepsYAML string) []byte {
	return []byte(fmt.Sprintf(`
version: 3
name: test
description: test
on:
  - repository: github.com/sourcegraph/sourcegraph
steps:
%s
changesetTemplate:
  title: test
  body: test
  branch: test
  commit:
    message: test
`, stepsYAML))
}

func TestParseBatchSpec_v3_codingAgentRequiresImage(t *testing.T) {
	_, err := ParseBatchSpec(v3SpecWith("  - codingAgent:\n      type: codex\n      prompt: do the thing"))
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "requires an image") {
		t.Errorf("error should mention missing image, got: %v", err)
	}
}

func TestParseBatchSpec_v3_codingAgentStep(t *testing.T) {
	got, err := ParseBatchSpec(v3SpecWith("  - codingAgent:\n      type: codex\n      prompt: do the thing\n    image: alpine:3"))
	if err != nil {
		t.Fatalf("ParseBatchSpec failed: %v", err)
	}
	if len(got.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(got.Steps))
	}
	step := got.Steps[0]
	if step.CodingAgent == nil {
		t.Fatal("expected step.CodingAgent to be set")
	}
	if step.CodingAgent.Type != CodingAgentTypeCodex {
		t.Errorf("CodingAgent.Type: got %q want %q", step.CodingAgent.Type, CodingAgentTypeCodex)
	}
	if step.CodingAgent.Prompt != "do the thing" {
		t.Errorf("CodingAgent.Prompt: got %q want %q", step.CodingAgent.Prompt, "do the thing")
	}
	if step.Image != "alpine:3" {
		t.Errorf("Step.Image: got %q want %q", step.Image, "alpine:3")
	}
}
