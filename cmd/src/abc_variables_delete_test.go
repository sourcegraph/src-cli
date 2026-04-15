package main

import (
	"testing"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
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
	if variableNames[0] != "approval" || variableNames[1] != "checkpoints" || variableNames[2] != "prompt" {
		t.Fatalf("variableNames = %#v, want [approval checkpoints prompt]", variableNames)
	}
}

func TestParseABCVariableNamesRequiresAtLeastOneName(t *testing.T) {
	t.Parallel()

	_, err := parseABCVariableNames(nil, nil)
	if err == nil {
		t.Fatal("parseABCVariableNames returned nil error, want usage error")
	}
	if _, ok := err.(*cmderrors.UsageError); !ok {
		t.Fatalf("parseABCVariableNames error = %T, want *cmderrors.UsageError", err)
	}
}

func TestParseABCVariableNamesRejectsEmptyNames(t *testing.T) {
	t.Parallel()

	_, err := parseABCVariableNames([]string{"approval", ""}, nil)
	if err == nil {
		t.Fatal("parseABCVariableNames returned nil error, want usage error")
	}
	if _, ok := err.(*cmderrors.UsageError); !ok {
		t.Fatalf("parseABCVariableNames error = %T, want *cmderrors.UsageError", err)
	}
}
