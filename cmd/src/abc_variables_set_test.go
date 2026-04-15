package main

import (
	"testing"
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
		abcVariableArgs{"checkpoints=[1,2,3]"},
	)
	if err != nil {
		t.Fatalf("parseABCVariables returned error: %v", err)
	}

	if len(variables) != 3 {
		t.Fatalf("len(variables) = %d, want 3", len(variables))
	}

	if variables[0].Key != "prompt" || variables[0].Value != `"tighten the review criteria"` {
		t.Fatalf("variables[0] = %#v, want prompt string variable", variables[0])
	}

	if variables[1].Key != "title" || variables[1].Value != `"test"` {
		t.Fatalf("variables[1] = %#v, want quoted test string", variables[1])
	}

	if variables[2].Key != "checkpoints" || variables[2].Value != "[1,2,3]" {
		t.Fatalf("variables[2] = %#v, want compact JSON array", variables[2])
	}
}
