package executor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/batches/template"
)

func TestParseContainerTempPath(t *testing.T) {
	for _, valid := range []string{"/tmp/tmp.abc-123_456", "/tmp/tmp.abc-123_456\n"} {
		t.Run("valid_"+valid, func(t *testing.T) {
			got, err := parseContainerTempPath(valid)
			require.NoError(t, err)
			require.Equal(t, "/tmp/tmp.abc-123_456", got)
		})
	}

	for _, invalid := range []string{
		"",
		"\n",
		"relative/path\n",
		"/tmp/first\n/tmp/second\n",
		"/tmp/path\r\n",
		"/tmp/x,source=/var/run/docker.sock,target=/var/run/docker.sock\n",
		"/tmp/path with spaces\n",
		`/tmp/path"quoted`,
		`/tmp/path\backslash`,
	} {
		t.Run("invalid_"+invalid, func(t *testing.T) {
			_, err := parseContainerTempPath(invalid)
			require.Error(t, err)
		})
	}
}

func TestProbeImageForShellRejectsMountInjection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a shell script as a fake docker executable")
	}

	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	err := os.WriteFile(docker, []byte("#!/bin/sh\nprintf '%s\\n' '/tmp/x,source=/var/run/docker.sock,target=/var/run/docker.sock'\n"), 0o755)
	require.NoError(t, err)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, _, err = probeImageForShell(context.Background(), "malicious-image")
	require.Error(t, err)
	require.Contains(t, err.Error(), "mktemp returned invalid path")
}

func TestDockerBindMountRejectsMountGrammar(t *testing.T) {
	_, err := dockerBindMount("/tmp/script", "/tmp/x,source=/var/run/docker.sock")
	require.Error(t, err)
}

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
