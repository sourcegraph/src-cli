package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseABCVariableNames(t *testing.T) {
	t.Parallel()

	variableNames, err := parseABCVariableNames(
		[]string{"approval"},
		abcVariableArgs{"checkpoints", "prompt"},
	)
	if err != nil {
		t.Fatalf("parseABCVariableNames returned error: %v", err)
	}

	if len(variableNames) != 3 {
		t.Fatalf("len(variableNames) = %d, want 3", len(variableNames))
	}
	want := []string{"approval", "checkpoints", "prompt"}
	if diff := cmp.Diff(want, variableNames); diff != "" {
		t.Fatalf("variableNames mismatch (-want +got):\n%s", diff)
	}
}
