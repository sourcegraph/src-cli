package main

import (
	"context"
	"strings"
	"testing"

	mockapi "github.com/sourcegraph/src-cli/internal/api/mock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestListRepositoriesSkipsRepositoryWhenDefaultBranchCannotBeResolved(t *testing.T) {
	client := new(mockapi.Client)
	request := &mockapi.Request{Response: `{
		"data": {
			"repositories": {
				"nodes": [
					{
						"id": "UmVwb3NpdG9yeTox",
						"name": "github.com/sourcegraph/ok",
						"url": "/github.com/sourcegraph/ok",
						"description": "",
						"language": "Go",
						"createdAt": "2026-03-31T00:00:00Z",
						"updatedAt": null,
						"externalRepository": {
							"id": "RXh0ZXJuYWxSZXBvc2l0b3J5OjE=",
							"serviceType": "github",
							"serviceID": "https://github.com/"
						},
						"defaultBranch": {
							"name": "refs/heads/main",
							"displayName": "main"
						},
						"viewerCanAdminister": false,
						"keyValuePairs": []
					},
					{
						"id": "UmVwb3NpdG9yeToy",
						"name": "github.com/sourcegraph/broken",
						"url": "/github.com/sourcegraph/broken",
						"description": "",
						"language": "Go",
						"createdAt": "2026-03-31T00:00:00Z",
						"updatedAt": null,
						"externalRepository": {
							"id": "RXh0ZXJuYWxSZXBvc2l0b3J5OjI=",
							"serviceType": "github",
							"serviceID": "https://github.com/"
						},
						"defaultBranch": null,
						"viewerCanAdminister": false,
						"keyValuePairs": []
					}
				]
			}
		},
		"errors": [
			{
				"message": "failed to resolve HEAD for github.com/sourcegraph/broken",
				"path": ["repositories", "nodes", 1, "defaultBranch"]
			}
		]
	}`}

	client.On(
		"NewRequest",
		mock.MatchedBy(func(query string) bool {
			return strings.Contains(query, "defaultBranch")
		}),
		mock.MatchedBy(func(vars map[string]any) bool {
			indexed, ok := vars["indexed"].(bool)
			if !ok || !indexed {
				return false
			}
			notIndexed, ok := vars["notIndexed"].(bool)
			return ok && !notIndexed
		}),
	).Return(request).Once()

	request.On("DoRaw", context.Background(), mock.Anything).
		Return(true, nil).
		Once()

	repos, skipped, err := listRepositories(context.Background(), client, reposListOptions{
		first:      1000,
		cloned:     true,
		notCloned:  true,
		indexed:    true,
		notIndexed: false,
		orderBy:    "REPOSITORY_NAME",
	})
	require.NoError(t, err)
	require.Len(t, repos, 1)
	require.Len(t, skipped, 1)
	require.Equal(t, "github.com/sourcegraph/ok", repos[0].Name)
	require.Equal(t, "github.com/sourcegraph/broken", skipped[0].Name)
	client.AssertExpectations(t)
	request.AssertExpectations(t)
}
