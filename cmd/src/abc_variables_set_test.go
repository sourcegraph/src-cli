package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// If we were to do a json marshalling roundtrip, it may break large integer literals.
// This test is here to demonstrate that the compaction approach is working well.
func TestMarshalABCVariableValuePreservesLargeIntegerLiteral(t *testing.T) {
	t.Parallel()

	value, remove, err := marshalABCVariableValue("9007199254740993")
	if err != nil {
		t.Fatalf("marshalABCVariableValue returned error: %v", err)
	}
	if remove {
		t.Fatal("marshalABCVariableValue unexpectedly marked value for removal")
	}
	if value != "9007199254740993" {
		t.Fatalf("marshalABCVariableValue = %q, want %q", value, "9007199254740993")
	}
}

func TestParseABCVariables(t *testing.T) {
	t.Parallel()

	variables, err := parseABCVariables(
		[]string{"prompt=tighten the review criteria", `title="test"`},
		[]string{"checkpoints=[1,2,3]"},
	)
	if err != nil {
		t.Fatalf("parseABCVariables returned error: %v", err)
	}

	if diff := cmp.Diff(variables, map[string]string{
		"prompt":      "\"tighten the review criteria\"",
		"title":       "\"test\"",
		"checkpoints": "[1,2,3]",
	}); diff != "" {
		t.Errorf("err: %v", diff)
	}
}
