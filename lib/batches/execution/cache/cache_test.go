package cache

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/batches/env"
)

func TestKeyer_Key_PerJobEnvVarsIgnored(t *testing.T) {
	var stepEnv env.Environment
	require.NoError(t, json.Unmarshal(
		[]byte(`["SRC_EXECUTOR_JOB_TOKEN", "SRC_EXECUTOR_JOB_ID", "SRC_EXECUTOR_NAME"]`),
		&stepEnv,
	))
	step := batches.Step{Run: "foo", Env: stepEnv}
	repo := batches.Repository{ID: "r", Name: "r"}

	unset, err := (&CacheKey{Repository: repo, Steps: []batches.Step{step}, StepIndex: 0}).Key()
	require.NoError(t, err)
	resolved, err := (&CacheKey{Repository: repo, Steps: []batches.Step{step}, StepIndex: 0, GlobalEnv: []string{
		"SRC_EXECUTOR_JOB_TOKEN=tok",
		"SRC_EXECUTOR_JOB_ID=42",
		"SRC_EXECUTOR_NAME=executor-abc",
	}}).Key()
	require.NoError(t, err)
	require.Equal(t, unset, resolved)
}
