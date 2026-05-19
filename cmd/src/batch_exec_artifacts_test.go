package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sourcegraph/sourcegraph/lib/batches/execution"
)

func TestBatchArtifactUploaderUploadAddsMetadata(t *testing.T) {
	const artifactContents = "artifact contents"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %q", r.Method)
		}
		if r.URL.Path != "/.executors/queue/batches/jobs/42/artifacts/stdout" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		if got := r.Header.Get("X-Sourcegraph-Executor-Name"); got != "executor" {
			t.Fatalf("unexpected executor header %q", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != artifactContents {
			t.Fatalf("unexpected body %q", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(execution.ArtifactReference{ObjectStorageKey: "key"})
	}))
	t.Cleanup(server.Close)

	endpointURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	uploader := &batchArtifactUploader{
		endpointURL:  endpointURL,
		jobID:        42,
		jobToken:     "token",
		executorName: "executor",
		client:       server.Client(),
	}

	ref, err := uploader.Upload(context.Background(), "stdout", strings.NewReader(artifactContents))
	if err != nil {
		t.Fatal(err)
	}

	if ref.ObjectStorageKey != "key" {
		t.Fatalf("unexpected object storage key %q", ref.ObjectStorageKey)
	}
	if ref.Size != int64(len(artifactContents)) {
		t.Fatalf("unexpected size %d", ref.Size)
	}
}
