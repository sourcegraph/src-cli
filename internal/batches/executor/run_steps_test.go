package executor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sourcegraph/sourcegraph/lib/batches/template"
)

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
