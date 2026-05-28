package batches

import (
	"testing"
)

// TestParseBatchSpec_v3_imageMirroredToContainer ensures that in v3 specs the
// step-level `image:` field is mirrored into Step.Container so executor
// consumers that read step.Container keep working.
func TestParseBatchSpec_v3_imageMirroredToContainer(t *testing.T) {
	spec := []byte(`
version: 3
name: test
description: test
on:
  - repository: github.com/sourcegraph/sourcegraph
steps:
  - run: echo hi
    image: alpine:3
changesetTemplate:
  title: test
  body: test
  branch: test
  commit:
    message: test
`)
	got, err := ParseBatchSpec(spec)
	if err != nil {
		t.Fatalf("ParseBatchSpec failed: %v", err)
	}
	if len(got.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(got.Steps))
	}
	if got.Steps[0].Image != "alpine:3" {
		t.Errorf("Step.Image: got %q want %q", got.Steps[0].Image, "alpine:3")
	}
	if got.Steps[0].Container != "alpine:3" {
		t.Errorf("Step.Container (mirrored from image): got %q want %q", got.Steps[0].Container, "alpine:3")
	}
}

// TestParseBatchSpec_v3_codingAgentStep ensures that a v3 spec with a
// codingAgent step parses with the expected typed step.
func TestParseBatchSpec_v3_codingAgentStep(t *testing.T) {
	spec := []byte(`
version: 3
name: test
description: test
on:
  - repository: github.com/sourcegraph/sourcegraph
steps:
  - codingAgent:
      type: codex
      prompt: do the thing
    image: alpine:3
changesetTemplate:
  title: test
  body: test
  branch: test
  commit:
    message: test
`)
	got, err := ParseBatchSpec(spec)
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
	if step.Container != "alpine:3" {
		t.Errorf("Step.Container (mirrored from image): got %q want %q", step.Container, "alpine:3")
	}
}
