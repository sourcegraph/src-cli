package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/sourcegraph/sourcegraph/lib/batches/execution"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const maxInlineArtifactSize = math.MaxInt64

type ArtifactUploader interface {
	Upload(ctx context.Context, artifactKey string, r io.Reader) (execution.Artifact, error)
}

type stepOutput struct {
	stdout *artifactOutput
	stderr *artifactOutput
}

func newStepOutput(dir string, threshold int64) (*stepOutput, error) {
	stdout, err := newArtifactOutput(dir, "stdout-*", threshold)
	if err != nil {
		return nil, err
	}
	stderr, err := newArtifactOutput(dir, "stderr-*", threshold)
	if err != nil {
		stdout.cleanup()
		return nil, err
	}
	return &stepOutput{stdout: stdout, stderr: stderr}, nil
}

func (o *stepOutput) cleanup() {
	o.stdout.cleanup()
	o.stderr.cleanup()
}

type artifactOutput struct {
	file      *os.File
	buf       bytes.Buffer
	size      int64
	threshold int64
}

func newArtifactOutput(dir, pattern string, threshold int64) (*artifactOutput, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, errors.Wrap(err, "creating artifact output file")
	}
	return &artifactOutput{file: file, threshold: threshold}, nil
}

func (o *artifactOutput) writer() io.Writer { return o }

func (o *artifactOutput) Write(p []byte) (int, error) {
	n, err := o.file.Write(p)
	o.size += int64(n)
	if o.size <= o.threshold {
		_, _ = o.buf.Write(p[:n])
	}
	return n, err
}

func (o *artifactOutput) inline() string {
	if o.size > o.threshold {
		return ""
	}
	return o.buf.String()
}

func (o *artifactOutput) shouldUpload(threshold int64) bool {
	return o.size > threshold
}

func (o *artifactOutput) reader() (io.Reader, error) {
	if _, err := o.file.Seek(0, io.SeekStart); err != nil {
		return nil, errors.Wrap(err, "rewinding artifact output file")
	}
	return o.file, nil
}

func (o *artifactOutput) cleanup() {
	if o.file == nil {
		return
	}
	name := o.file.Name()
	_ = o.file.Close()
	_ = os.Remove(name)
	o.file = nil
}

func uploadStepArtifacts(ctx context.Context, uploader ArtifactUploader, threshold int64, stepIndex int, result *execution.AfterStepResult, output *stepOutput) error {
	defer output.cleanup()
	result.Artifacts = make(map[string]execution.Artifact)

	if output.stdout.shouldUpload(threshold) {
		ref, err := uploadArtifactOutput(ctx, uploader, artifactKey(stepIndex, execution.ArtifactStdout), output.stdout)
		if err != nil {
			return err
		}
		result.Artifacts[execution.ArtifactStdout] = ref
	}

	if output.stderr.shouldUpload(threshold) {
		ref, err := uploadArtifactOutput(ctx, uploader, artifactKey(stepIndex, execution.ArtifactStderr), output.stderr)
		if err != nil {
			return err
		}
		result.Artifacts[execution.ArtifactStderr] = ref
	}

	if int64(len(result.Diff)) > threshold {
		ref, err := uploader.Upload(ctx, artifactKey(stepIndex, execution.ArtifactDiff), bytes.NewReader(result.Diff))
		if err != nil {
			return errors.Wrap(err, "uploading diff artifact")
		}
		result.Artifacts[execution.ArtifactDiff] = ref
		result.Diff = nil
	}

	return nil
}

func uploadArtifactOutput(ctx context.Context, uploader ArtifactUploader, key string, output *artifactOutput) (execution.Artifact, error) {
	reader, err := output.reader()
	if err != nil {
		return execution.Artifact{}, err
	}
	ref, err := uploader.Upload(ctx, key, reader)
	if err != nil {
		return execution.Artifact{}, errors.Wrapf(err, "uploading %s artifact", key)
	}
	return ref, nil
}

func artifactKey(stepIndex int, name string) string {
	return fmt.Sprintf("step-%d-%s", stepIndex, name)
}
