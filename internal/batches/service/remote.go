package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

var ErrServerSideBatchChangesUnsupported = errors.New("server side batch changes are not available on this Sourcegraph instance")

const upsertEmptyBatchChangeQuery = `
mutation UpsertEmptyBatchChange(
	$name: String!
	$namespace: ID!
) {
	upsertEmptyBatchChange(
		name: $name,
		namespace: $namespace
	) {
		name
		id
	}
}
`

func (svc *Service) UpsertBatchChange(
	ctx context.Context,
	name string,
	namespaceID string,
) (string, error) {
	var resp struct {
		UpsertEmptyBatchChange struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"upsertEmptyBatchChange"`
	}

	if ok, err := svc.client.NewRequest(upsertEmptyBatchChangeQuery, map[string]interface{}{
		"name":      name,
		"namespace": namespaceID,
	}).Do(ctx, &resp); err != nil || !ok {
		return "", "", err
	}

	return resp.UpsertEmptyBatchChange.ID, resp.UpsertEmptyBatchChange.Name, nil
}

const createBatchSpecFromRawQuery = `
mutation CreateBatchSpecFromRaw(
    $batchSpec: String!,
    $namespace: ID!,
    $allowIgnored: Boolean!,
    $allowUnsupported: Boolean!,
    $noCache: Boolean!,
    $batchChange: ID!,
) {
    createBatchSpecFromRaw(
        batchSpec: $batchSpec,
        namespace: $namespace,
        allowIgnored: $allowIgnored,
        allowUnsupported: $allowUnsupported,
        noCache: $noCache,
        batchChange: $batchChange,
    ) {
        id
    }
}
`

func (svc *Service) CreateBatchSpecFromRaw(
	ctx context.Context,
	batchSpec string,
	namespaceID string,
	allowIgnored bool,
	allowUnsupported bool,
	noCache bool,
	batchChange string,
) (string, error) {
	var resp struct {
		CreateBatchSpecFromRaw struct {
			ID string `json:"id"`
		} `json:"createBatchSpecFromRaw"`
	}

	if ok, err := svc.client.NewRequest(createBatchSpecFromRawQuery, map[string]interface{}{
		"batchSpec":        batchSpec,
		"namespace":        namespaceID,
		"allowIgnored":     allowIgnored,
		"allowUnsupported": allowUnsupported,
		"noCache":          noCache,
		"batchChange":      batchChange,
	}).Do(ctx, &resp); err != nil || !ok {
		return "", err
	}

	return resp.CreateBatchSpecFromRaw.ID, nil
}

// UploadMounts uploads file mounts to the server.
func (svc *Service) UploadMounts(workingDir string, batchSpecID string, steps []batches.Step) error {
	if err := svc.areServerSideBatchChangesSupported(); err != nil {
		return err
	}

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)

	var count int
	for _, step := range steps {
		for _, mount := range step.Mount {
			total, err := handlePath(w, workingDir, mount.Path, count)
			if err != nil {
				return err
			}
			count += total
		}
	}

	if err := w.WriteField("count", strconv.Itoa(count)); err != nil {
		return err
	}

	// Honestly, the most import thing to do. This adds the closing boundary to the request.
	if err := w.Close(); err != nil {
		return err
	}

	request, err := svc.client.NewHTTPRequest(context.Background(), http.MethodPost, fmt.Sprintf(".api/batches/mount/%s", batchSpecID), body)
	if err != nil {
		return err
	}
	request.Header.Add("Content-Type", w.FormDataContentType())

	resp, err := svc.client.Do(request)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		p, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return errors.New(string(p))
	}
	return nil
}

func handlePath(w *multipart.Writer, workingDir, mountPath string, offset int) (int, error) {
	total := 0
	actualFilePath := filepath.Join(workingDir, mountPath)
	info, err := os.Stat(actualFilePath)
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		dir, err := os.ReadDir(actualFilePath)
		if err != nil {
			return 0, err
		}
		for _, dirEntry := range dir {
			totalFiles, err := handlePath(w, workingDir, filepath.Join(mountPath, dirEntry.Name()), offset+total)
			if err != nil {
				return 0, err
			}
			total += totalFiles
		}
	} else {
		if err = createFormFile(w, workingDir, mountPath, offset+total); err != nil {
			return 0, err
		}
		total++
	}
	return total, nil
}

func createFormFile(w *multipart.Writer, workingDir string, mountPath string, index int) error {
	// TODO: limit file size
	f, err := os.Open(filepath.Join(workingDir, mountPath))
	if err != nil {
		return err
	}
	defer f.Close()

	filePath, fileName := filepath.Split(mountPath)
	trimmedPath := strings.Trim(strings.TrimSuffix(filePath, string(filepath.Separator)), ".")
	if err = w.WriteField(fmt.Sprintf("filepath_%d", index), trimmedPath); err != nil {
		return err
	}
	fileInfo, err := f.Stat()
	if err != nil {
		return err
	}
	if err = w.WriteField(fmt.Sprintf("filemod_%d", index), fileInfo.ModTime().UTC().String()); err != nil {
		return err
	}

	part, err := w.CreateFormFile(fmt.Sprintf("file_%d", index), fileName)
	if err != nil {
		return err
	}
	if _, err = io.Copy(part, f); err != nil {
		return err
	}
	return nil
}

const executeBatchSpecQuery = `
mutation ExecuteBatchSpec($batchSpec: ID!, $noCache: Boolean!) {
    executeBatchSpec(batchSpec: $batchSpec, noCache: $noCache) {
        id
    }
}
`

func (svc *Service) ExecuteBatchSpec(
	ctx context.Context,
	batchSpecID string,
	noCache bool,
) (string, error) {
	var resp struct {
		ExecuteBatchSpec struct {
			ID string `json:"id"`
		} `json:"executeBatchSpec"`
	}

	if ok, err := svc.client.NewRequest(executeBatchSpecQuery, map[string]interface{}{
		"batchSpec": batchSpecID,
		"noCache":   noCache,
	}).Do(ctx, &resp); err != nil || !ok {
		return "", err
	}

	return resp.ExecuteBatchSpec.ID, nil
}

const batchSpecWorkspaceResolutionQuery = `
query BatchSpecWorkspaceResolution($batchSpec: ID!) {
    node(id: $batchSpec) {
        ... on BatchSpec {
            workspaceResolution {
                failureMessage
                state
            }
        }
    }
}
`

type BatchSpecWorkspaceResolution struct {
	FailureMessage string `json:"failureMessage"`
	State          string `json:"state"`
}

func (svc *Service) GetBatchSpecWorkspaceResolution(ctx context.Context, id string) (*BatchSpecWorkspaceResolution, error) {
	var resp struct {
		Node struct {
			WorkspaceResolution BatchSpecWorkspaceResolution `json:"workspaceResolution"`
		} `json:"node"`
	}

	if ok, err := svc.client.NewRequest(batchSpecWorkspaceResolutionQuery, map[string]interface{}{
		"batchSpec": id,
	}).Do(ctx, &resp); err != nil || !ok {
		return nil, err
	}

	return &resp.Node.WorkspaceResolution, nil
}
