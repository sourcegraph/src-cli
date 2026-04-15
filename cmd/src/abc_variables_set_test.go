package main

import "testing"

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
