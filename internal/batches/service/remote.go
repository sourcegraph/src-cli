package service

import (
	"context"

	"github.com/cockroachdb/errors"
)

var ErrServerSideBatchChangesUnsupported = errors.New("server side batch changes are not available on this Sourcegraph instance")

func (svc *Service) areServerSideBatchChangesSupported() error {
	if !svc.features.ServerSideBatchChanges {
		return ErrServerSideBatchChangesUnsupported
	}
	return nil
}

const batchSpecIDByNameQuery = `
query BatchSpecIDByName($namespace: ID!, $name: String!) {
    batchChange(namespace: $namespace, name: $name) {
        currentSpec {
            id
        }
    }
}
`

func (svc *Service) GetBatchSpecIDByName(ctx context.Context, namespace, name string) (string, error) {
	var resp struct {
		BatchChange struct {
			CurrentSpec struct {
				ID string `json:"id"`
			} `json:"currentSpec"`
		} `json:"batchChange"`
	}

	if ok, err := svc.client.NewRequest(batchSpecIDByNameQuery, map[string]interface{}{
		"namespace": namespace,
		"name":      name,
	}).Do(ctx, &resp); err != nil || !ok {
		return "", err
	}

	return resp.BatchChange.CurrentSpec.ID, nil
}

const createBatchSpecFromRawQuery = `
mutation CreateBatchSpecFromRaw(
    $batchSpec: String!,
    $namespace: ID!,
    $allowIgnored: Boolean!,
    $allowUnsupported: Boolean!,
    $noCache: Boolean!,
) {
    createBatchSpecFromRaw(
        batchSpec: $batchSpec,
        namespace: $namespace,
        allowIgnored: $allowIgnored,
        allowUnsupported: $allowUnsupported,
        noCache: $noCache,
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
) (string, error) {
	if err := svc.areServerSideBatchChangesSupported(); err != nil {
		return "", err
	}

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
	}).Do(ctx, &resp); err != nil || !ok {
		return "", err
	}

	return resp.CreateBatchSpecFromRaw.ID, nil
}

const replaceBatchSpecInputQuery = `
mutation ReplaceBatchSpecInput(
    $previousSpec: ID!,
    $batchSpec: String!,
    $allowIgnored: Boolean!,
    $allowUnsupported: Boolean!,
    $noCache: Boolean!,
) {
    replaceBatchSpecInput(
        previousSpec: $previousSpec,
        batchSpec: $batchSpec,
        allowIgnored: $allowIgnored,
        allowUnsupported: $allowUnsupported,
        noCache: $noCache,
    ) {
        id
    }
}
`

func (svc *Service) ReplaceBatchSpecInput(
	ctx context.Context,
	previousSpecID string,
	batchSpec string,
	allowIgnored bool,
	allowUnsupported bool,
	noCache bool,
) (string, error) {
	if err := svc.areServerSideBatchChangesSupported(); err != nil {
		return "", err
	}

	var resp struct {
		ReplaceBatchSpecInput struct {
			ID string `json:"id"`
		} `json:"replaceBatchSpecInput"`
	}

	if ok, err := svc.client.NewRequest(replaceBatchSpecInputQuery, map[string]interface{}{
		"previousSpec":     previousSpecID,
		"batchSpec":        batchSpec,
		"allowIgnored":     allowIgnored,
		"allowUnsupported": allowUnsupported,
		"noCache":          noCache,
	}).Do(ctx, &resp); err != nil || !ok {
		return "", err
	}

	return resp.ReplaceBatchSpecInput.ID, nil
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
