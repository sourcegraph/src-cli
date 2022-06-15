package docker

import (
	"bytes"
	"context"
	"runtime"
	"strconv"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	"github.com/sourcegraph/src-cli/internal/exec"
)

// NCPU returns the number of CPU cores available to Docker.
func NCPU(ctx context.Context) (int, error) {
	ncpu := runtime.GOMAXPROCS(0)

	dctx, cancel, err := withFastCommandContext(ctx)
	if err != nil {
		return ncpu, err
	}
	defer cancel()

	args := []string{"info", "--format", "{{ .NCPU }}"}
	out, err := exec.CommandContext(dctx, "docker", args...).CombinedOutput()
	if errors.IsDeadlineExceeded(err) || errors.IsDeadlineExceeded(dctx.Err()) {
		return ncpu, newFastCommandTimeoutError(dctx, args...)
	} else if err != nil {
		return ncpu, err
	}

	dcpu, err := strconv.Atoi(string(bytes.TrimSpace(out)))
	if err != nil {
		return ncpu, err
	}

	return dcpu, nil
}
