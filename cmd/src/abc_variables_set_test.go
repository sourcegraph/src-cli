package main

import "testing"

func TestMarshalABCVariableValue(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantValue  string
		wantRemove bool
	}{
		{
			name:       "plain string",
			raw:        "hello",
			wantValue:  `"hello"`,
			wantRemove: false,
		},
		{
			name:       "number literal",
			raw:        "42",
			wantValue:  `42`,
			wantRemove: false,
		},
		{
			name:       "boolean literal",
			raw:        "true",
			wantValue:  `true`,
			wantRemove: false,
		},
		{
			name:       "null removes variable",
			raw:        "null",
			wantValue:  `null`,
			wantRemove: true,
		},
		{
			name:       "quoted null stays a string",
			raw:        "\"null\"",
			wantValue:  "\"null\"",
			wantRemove: false,
		},
		{
			name:       "object literal",
			raw:        "{\"retries\":3,\"notify\":true}",
			wantValue:  "{\"notify\":true,\"retries\":3}",
			wantRemove: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotValue, gotRemove, err := marshalABCVariableValue(test.raw)
			if err != nil {
				t.Fatalf("marshalABCVariableValue returned error: %s", err)
			}
			if gotValue != test.wantValue {
				t.Fatalf("value = %q, want %q", gotValue, test.wantValue)
			}
			if gotRemove != test.wantRemove {
				t.Fatalf("remove = %v, want %v", gotRemove, test.wantRemove)
			}
		})
	}
}
