package service

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/sourcegraph/src-cli/internal/api"

	"github.com/sourcegraph/sourcegraph/lib/batches"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

var ErrServerSideBatchChangesUnsupported = errors.New("server side batch changes are not available on this Sourcegraph instance")

func (svc *Service) areServerSideBatchChangesSupported() error {
	if !svc.features.ServerSideBatchChanges {
		return ErrServerSideBatchChangesUnsupported
	}
	return nil
}

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
) (string, string, error) {
	if err := svc.areServerSideBatchChangesSupported(); err != nil {
		return "", "", err
	}

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
        uploadToken
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
	if err := svc.areServerSideBatchChangesSupported(); err != nil {
		return "", err
	}

	var resp struct {
		CreateBatchSpecFromRaw struct {
			ID          string `json:"id"`
			UploadToken string `json:"uploadToken"`
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

func (svc *Service) UploadMounts(dir string, batchSpecID string, steps []batches.Step) error {
	if err := svc.areServerSideBatchChangesSupported(); err != nil {
		return err
	}

	for _, step := range steps {
		// TODO bulk + parallel
		for _, mount := range step.Mount {
			if err := uploadMount(svc.client, batchSpecID, filepath.Join(dir, mount.Path)); err != nil {
				return err
			}
		}
	}
	return nil
}

func uploadMount(client api.Client, batchSpecID string, mountPath string) error {
	info, err := os.Stat(mountPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		dir, err := os.ReadDir(mountPath)
		if err != nil {
			return err
		}
		for _, dirEntry := range dir {
			if err = uploadMount(client, batchSpecID, filepath.Join(mountPath, dirEntry.Name())); err != nil {
				return err
			}
		}
	} else {
		if err = uploadFile(client, batchSpecID, mountPath); err != nil {
			return err
		}
	}
	return nil
}

func uploadFile(client api.Client, batchSpecID string, path string) error {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if err = w.WriteField("count", "1"); err != nil {
		return err
	}
	if err = w.WriteField("batchSpecID", batchSpecID); err != nil {
		return err
	}

	part, err := w.CreateFormFile("file_0", "foo/bar/updater.py")
	if err != nil {
		return err
	}
	if _, err = io.Copy(part, f); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}

	request, err := client.NewHTTPRequest(context.Background(), http.MethodPost, filepath.Join(".api/batches/mount", batchSpecID), body)
	if err != nil {
		return err
	}
	request.Header.Add("Content-Type", w.FormDataContentType())

	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return errors.New("failed")
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
	if err := svc.areServerSideBatchChangesSupported(); err != nil {
		return "", err
	}

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
	if err := svc.areServerSideBatchChangesSupported(); err != nil {
		return nil, err
	}

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
