package batches

import (
	"encoding/json"
	"strings"
	"testing"
)

// Step JSON must always emit `container` (never `image`) so prep-side cache
// keys computed by marshaling Step match src-cli's serialization.
func TestStep_MarshalJSON_canonicalizesImageToContainer(t *testing.T) {
	v3FromImage, err := json.Marshal(Step{Image: "alpine:3", Run: "echo hi"})
	if err != nil {
		t.Fatalf("marshal v3-shaped step: %v", err)
	}
	v1FromContainer, err := json.Marshal(Step{Container: "alpine:3", Run: "echo hi"})
	if err != nil {
		t.Fatalf("marshal v1-shaped step: %v", err)
	}
	if string(v3FromImage) != string(v1FromContainer) {
		t.Errorf("canonical JSON differs:\n  v3 image:     %s\n  v1 container: %s", v3FromImage, v1FromContainer)
	}
	var out map[string]any
	if err := json.Unmarshal(v3FromImage, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := out["image"]; ok {
		t.Errorf("expected no image key in canonical JSON, got %s", v3FromImage)
	}
	if got, _ := out["container"].(string); got != "alpine:3" {
		t.Errorf("container: got %v want alpine:3 (full=%s)", out["container"], v3FromImage)
	}
}

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

// A v3 codingAgent step without image: must be rejected at parse time so
// it doesn't fail later with an opaque empty-image error in the executor.
func TestParseBatchSpec_v3_codingAgentRequiresImage(t *testing.T) {
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
changesetTemplate:
  title: test
  body: test
  branch: test
  commit:
    message: test
`)
	_, err := ParseBatchSpec(spec)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "requires an image") &&
		!strings.Contains(err.Error(), "Must validate") {
		t.Errorf("error should mention missing image, got: %v", err)
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
