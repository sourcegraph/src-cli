package telemetry

import "testing"

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		feature string
		action  string
		wantErr bool
	}{
		{name: "valid simple", feature: "srcCli", action: "succeeded"},
		{name: "valid dotted feature", feature: "srcCli.batch.apply", action: "failed"},
		{name: "valid dashed feature", feature: "srcCli.code-intel", action: "succeeded"},
		{name: "empty feature", feature: "", action: "succeeded", wantErr: true},
		{name: "empty action", feature: "srcCli", action: "", wantErr: true},
		{name: "digit in feature", feature: "srcCli2", action: "succeeded", wantErr: true},
		{name: "underscore in action", feature: "srcCli", action: "did_it", wantErr: true},
		{name: "leading uppercase", feature: "SrcCli", action: "succeeded", wantErr: true},
		{name: "whitespace", feature: "srcCli search", action: "succeeded", wantErr: true},
		{name: "single char feature (too short for +)", feature: "s", action: "succeeded", wantErr: true},
		{name: "too long", feature: longName(70), action: "ok", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.feature, tt.action)
			if tt.wantErr && err == nil {
				t.Fatalf("Validate(%q, %q) = nil, want error", tt.feature, tt.action)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate(%q, %q) = %v, want nil", tt.feature, tt.action, err)
			}
		})
	}
}

func longName(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
