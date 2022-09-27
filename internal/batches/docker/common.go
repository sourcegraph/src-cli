package docker

import (
	"context"
	"os/exec"

	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// executeFastDockerCommand creates a fastCommandContext used to execute docker commands
// with a timeout for docker commands that are supposed to be fast (e.g docker info).
func executeFastDockerCommand(ctx context.Context, args ...string) ([]byte, error) {
	dctx, cancel, err := withFastCommandContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()

	out, err := exec.CommandContext(dctx, "docker", args...).CombinedOutput()
	if errors.IsDeadlineExceeded(err) || errors.IsDeadlineExceeded(dctx.Err()) {
		return nil, newFastCommandTimeoutError(dctx, args...)
	}

	return out, err
}
