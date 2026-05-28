package executor

import (
	"testing"

	codingagenttypes "github.com/sourcegraph/sourcegraph/lib/batches/codingagent/types"
)

// TestForwardCodingAgentEnv verifies that the model-provider auth env vars
// placed on the v1 CliStep by the Sourcegraph server are forwarded into the
// user container env for codingAgent steps.
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
