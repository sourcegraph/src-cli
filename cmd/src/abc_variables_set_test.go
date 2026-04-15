package main

import (
	"testing"

	"github.com/sourcegraph/src-cli/internal/cmderrors"
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

func TestMarshalABCVariableValueTreatsNullAsRemoval(t *testing.T) {
	t.Parallel()

	value, remove, err := marshalABCVariableValue("null")
	if err != nil {
		t.Fatalf("marshalABCVariableValue returned error: %v", err)
	}
	if !remove {
		t.Fatal("marshalABCVariableValue did not mark null for removal")
	}
	if value != "null" {
		t.Fatalf("marshalABCVariableValue = %q, want %q", value, "null")
	}
}

func TestParseABCVariables(t *testing.T) {
	t.Parallel()

	variables, err := parseABCVariables(
		[]string{"prompt=tighten the review criteria", `title="null"`},
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

	if variables[1].Key != "title" || variables[1].Value != `"null"` {
		t.Fatalf("variables[1] = %#v, want quoted null string", variables[1])
	}

	if variables[2].Key != "checkpoints" || variables[2].Value != "[1,2,3]" {
		t.Fatalf("variables[2] = %#v, want compact JSON array", variables[2])
	}
}

func TestParseABCVariablesRequiresAssignments(t *testing.T) {
	t.Parallel()

	_, err := parseABCVariables(nil, nil)
	if err == nil {
		t.Fatal("parseABCVariables returned nil error, want usage error")
	}
	if _, ok := err.(*cmderrors.UsageError); !ok {
		t.Fatalf("parseABCVariables error = %T, want *cmderrors.UsageError", err)
	}
}

func TestParseABCVariableRequiresNameValueFormat(t *testing.T) {
	t.Parallel()

	_, err := parseABCVariable("missing-separator")
	if err == nil {
		t.Fatal("parseABCVariable returned nil error, want usage error")
	}
	if _, ok := err.(*cmderrors.UsageError); !ok {
		t.Fatalf("parseABCVariable error = %T, want *cmderrors.UsageError", err)
	}
}

func TestParseABCVariableRejectsNullLiteral(t *testing.T) {
	t.Parallel()

	_, err := parseABCVariable("approval=null")
	if err == nil {
		t.Fatal("parseABCVariable returned nil error, want usage error")
	}
	if _, ok := err.(*cmderrors.UsageError); !ok {
		t.Fatalf("parseABCVariable error = %T, want *cmderrors.UsageError", err)
	}
}
