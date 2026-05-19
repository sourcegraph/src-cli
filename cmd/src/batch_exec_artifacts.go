package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/sourcegraph/src-cli/internal/batches/executor"

	"github.com/sourcegraph/sourcegraph/lib/batches/execution"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

const defaultArtifactUploadChunkSize = 1 << 20

type batchArtifactUploader struct {
	endpointURL  *url.URL
	jobID        int
	jobToken     string
	executorName string
	client       *http.Client
}

var _ executor.ArtifactUploader = (*batchArtifactUploader)(nil)

func newBatchArtifactUploaderFromEnv(endpointURL *url.URL) (*batchArtifactUploader, error) {
	jobID, _ := strconv.Atoi(os.Getenv("SRC_EXECUTOR_JOB_ID"))
	jobToken := os.Getenv("SRC_EXECUTOR_JOB_TOKEN")
	executorName := os.Getenv("SRC_EXECUTOR_NAME")

	configured := jobID != 0 || jobToken != "" || executorName != ""
	if !configured {
		return nil, nil
	}
	if jobID == 0 || jobToken == "" || executorName == "" {
		return nil, errors.New("artifact upload requires job ID, job token, and executor name")
	}

	return &batchArtifactUploader{
		endpointURL:  endpointURL,
		jobID:        jobID,
		jobToken:     jobToken,
		executorName: executorName,
		client:       http.DefaultClient,
	}, nil
}

func (u *batchArtifactUploader) Upload(ctx context.Context, artifactKey string, r io.Reader) (execution.ArtifactReference, error) {
	if strings.Contains(artifactKey, "/") || strings.Contains(artifactKey, "\\") || strings.Contains(artifactKey, "..") {
		return execution.ArtifactReference{}, errors.Newf("invalid artifact key %q", artifactKey)
	}
	sizeReader := &artifactSizeReader{r: r}

	url := u.endpointURL.JoinPath(
		".executors",
		"queue",
		"batches",
		"jobs",
		strconv.Itoa(u.jobID),
		"artifacts",
		artifactKey,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url.String(), sizeReader)
	if err != nil {
		return execution.ArtifactReference{}, err
	}
	req.Header.Set("Authorization", "Bearer "+u.jobToken)
	req.Header.Set("X-Sourcegraph-Executor-Name", u.executorName)

	resp, err := u.client.Do(req)
	if err != nil {
		return execution.ArtifactReference{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return execution.ArtifactReference{}, errors.Newf("artifact upload failed with status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var ref execution.ArtifactReference
	if err := json.NewDecoder(resp.Body).Decode(&ref); err != nil {
		return execution.ArtifactReference{}, errors.Wrap(err, "decoding artifact upload response")
	}
	if ref.URL == "" && ref.ObjectStorageKey == "" {
		return execution.ArtifactReference{}, errors.New("artifact upload response did not include a URL or object storage key")
	}
	ref.Size = sizeReader.size
	return ref, nil
}

type artifactSizeReader struct {
	r    io.Reader
	size int64
}

func (r *artifactSizeReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		r.size += int64(n)
	}
	return n, err
}
