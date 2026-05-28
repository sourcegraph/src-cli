package batches

import (
	"encoding/json"
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

func TestParseBatchSpec_v3_imageMirroredToContainer(t *testing.T) {
	got, err := ParseBatchSpec(v3SpecWith("  - run: echo hi\n    image: alpine:3"))
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
	if step.Container != "alpine:3" {
		t.Errorf("Step.Container (mirrored from image): got %q want %q", step.Container, "alpine:3")
	}
}
