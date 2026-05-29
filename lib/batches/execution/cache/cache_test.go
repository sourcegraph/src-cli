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
		[]byte(`["SRC_BATCHES_MODEL_PROVIDER_TOKEN", "SRC_BATCHES_JOB_ID"]`),
		&stepEnv,
	))
	step := batches.Step{Run: "foo", Env: stepEnv}
	repo := batches.Repository{ID: "r", Name: "r"}

	unset, err := (&CacheKey{Repository: repo, Steps: []batches.Step{step}, StepIndex: 0}).Key()
	require.NoError(t, err)
	resolved, err := (&CacheKey{Repository: repo, Steps: []batches.Step{step}, StepIndex: 0, GlobalEnv: []string{
		"SRC_BATCHES_MODEL_PROVIDER_TOKEN=tok",
		"SRC_BATCHES_JOB_ID=42",
	}}).Key()
	require.NoError(t, err)
	require.Equal(t, unset, resolved)
}
