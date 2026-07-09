package executor

import (
	"testing"

	"github.com/stretchr/testify/require"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/batches/template"
)

func TestCreateFilesToMount_RejectsCommaInTargetPath(t *testing.T) {
	step := batcheslib.Step{
		Files: map[string]string{
			"/tmp/x,source=/var/run/docker.sock,target=/var/run/docker.sock": "IGNORED",
		},
	}

	_, cleanup, err := createFilesToMount(t.TempDir(), step, &template.StepContext{})
	if cleanup != nil {
		cleanup()
	}
	require.Error(t, err)
	require.Contains(t, err.Error(), "contains invalid characters")
}

func TestRenderStepContainer(t *testing.T) {
	t.Run("static image", func(t *testing.T) {
		got, err := renderStepContainer("alpine:3", &template.StepContext{})
		require.NoError(t, err)
		require.Equal(t, "alpine:3", got)
	})

	t.Run("output image", func(t *testing.T) {
		got, err := renderStepContainer("${{ outputs.imageName }}", &template.StepContext{
			Outputs: map[string]any{"imageName": "alpine:3"},
		})
		require.NoError(t, err)
		require.Equal(t, "alpine:3", got)
	})

	t.Run("missing output", func(t *testing.T) {
		_, err := renderStepContainer("${{ outputs.imageName }}", &template.StepContext{})
		require.Error(t, err)
	})
}
