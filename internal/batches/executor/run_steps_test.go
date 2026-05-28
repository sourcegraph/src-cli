package executor

import (
	"slices"
	"testing"

	codingagenttypes "github.com/sourcegraph/sourcegraph/lib/batches/codingagent/types"
)

func TestRedactSensitiveEnv(t *testing.T) {
	in := map[string]string{
		codingagenttypes.ModelProviderTokenEnvVar: "tok-abc",
		"PATH":                                    "/bin",
	}
	out := redactSensitiveEnv(in)
	if got := out[codingagenttypes.ModelProviderTokenEnvVar]; got != redactedPlaceholder {
		t.Errorf("token: got %q want %q", got, redactedPlaceholder)
	}
	if got := out["PATH"]; got != "/bin" {
		t.Errorf("PATH should not be redacted: got %q", got)
	}
	if in[codingagenttypes.ModelProviderTokenEnvVar] != "tok-abc" {
		t.Errorf("input must not be mutated")
	}
}

func TestRedactSensitiveArgs(t *testing.T) {
	in := []string{
		"docker", "run",
		"-e", codingagenttypes.ModelProviderTokenEnvVar + "=tok-abc",
		"-e", codingagenttypes.JobIDEnvVar + "=job-123",
		"-e", "PATH=/bin",
		"--", "image:tag", "/script",
	}
	out := redactSensitiveArgs(in)
	if slices.Contains(out, codingagenttypes.ModelProviderTokenEnvVar+"=tok-abc") {
		t.Errorf("token value still present in args: %v", out)
	}
	if !slices.Contains(out, codingagenttypes.ModelProviderTokenEnvVar+"="+redactedPlaceholder) {
		t.Errorf("token not redacted in args: %v", out)
	}
	if !slices.Contains(out, codingagenttypes.JobIDEnvVar+"=job-123") {
		t.Errorf("job id should pass through: %v", out)
	}
}

func TestForwardCodingAgentEnv(t *testing.T) {
	cases := []struct {
		name      string
		globalEnv []string
		stepEnv   map[string]string
		want      map[string]string
	}{
		{
			name: "forwards both vars",
			globalEnv: []string{
				"PATH=/bin",
				codingagenttypes.ModelProviderTokenEnvVar + "=tok-abc",
				codingagenttypes.JobIDEnvVar + "=job-123",
			},
			stepEnv: map[string]string{},
			want: map[string]string{
				codingagenttypes.ModelProviderTokenEnvVar: "tok-abc",
				codingagenttypes.JobIDEnvVar:              "job-123",
			},
		},
		{
			name: "forwards only what is set",
			globalEnv: []string{
				codingagenttypes.JobIDEnvVar + "=job-456",
			},
			stepEnv: map[string]string{},
			want: map[string]string{
				codingagenttypes.JobIDEnvVar: "job-456",
			},
		},
		{
			name: "preserves preexisting step env and overwrites on match",
			globalEnv: []string{
				codingagenttypes.ModelProviderTokenEnvVar + "=from-global",
			},
			stepEnv: map[string]string{
				"OTHER": "x",
				codingagenttypes.ModelProviderTokenEnvVar: "from-step",
			},
			want: map[string]string{
				"OTHER": "x",
				codingagenttypes.ModelProviderTokenEnvVar: "from-global",
			},
		},
		{
			name:      "no-op when env not present",
			globalEnv: []string{"PATH=/bin"},
			stepEnv:   map[string]string{},
			want:      map[string]string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forwardCodingAgentEnv(tc.globalEnv, tc.stepEnv)
			if len(tc.stepEnv) != len(tc.want) {
				t.Fatalf("len mismatch: got %d want %d (got=%v want=%v)", len(tc.stepEnv), len(tc.want), tc.stepEnv, tc.want)
			}
			for k, v := range tc.want {
				if got := tc.stepEnv[k]; got != v {
					t.Errorf("env[%q]: got %q want %q", k, got, v)
				}
			}
		})
	}
}
