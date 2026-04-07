package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sourcegraph/src-cli/internal/api"
	mockapi "github.com/sourcegraph/src-cli/internal/api/mock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type expectedRepository struct {
	name          string
	defaultBranch GitRef
}

func TestListRepositoriesHandlesGraphQLErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		response     map[string]any
		wantRepos    []expectedRepository
		wantWarnings []string
		wantErr      string
	}{
		{
			name: "default branch warning preserves repositories",
			response: listRepositoriesResponse(
				[]map[string]any{
					repositoryNode("github.com/sourcegraph/ok", gitRefNode("refs/heads/main", "main")),
					repositoryNode("github.com/sourcegraph/broken", nil),
				},
				graphqlError("failed to resolve HEAD for github.com/sourcegraph/broken", "repositories", "nodes", 1, "defaultBranch"),
			),
			wantRepos: []expectedRepository{
				{name: "github.com/sourcegraph/ok", defaultBranch: GitRef{Name: "refs/heads/main", DisplayName: "main"}},
				{name: "github.com/sourcegraph/broken", defaultBranch: GitRef{}},
			},
			wantWarnings: []string{"failed to resolve HEAD for github.com/sourcegraph/broken"},
		},
		{
			name: "top-level warning with data is preserved as warning",
			response: listRepositoriesResponse(
				[]map[string]any{
					repositoryNode("github.com/sourcegraph/ok", gitRefNode("refs/heads/main", "main")),
				},
				graphqlError("listing timed out", "repositories"),
			),
			wantRepos: []expectedRepository{
				{name: "github.com/sourcegraph/ok", defaultBranch: GitRef{Name: "refs/heads/main", DisplayName: "main"}},
			},
			wantWarnings: []string{"listing timed out"},
		},
		{
			name: "errors without repositories are returned as hard errors",
			response: listRepositoriesResponse(
				nil,
				graphqlError("listing timed out", "repositories"),
			),
			wantErr: "listing timed out",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repos, warnings, err := runListRepositories(t, test.response)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Nil(t, repos)
				require.Nil(t, warnings)
				return
			}

			require.NoError(t, err)
			require.Len(t, warnings, len(test.wantWarnings))
			require.Len(t, repos, len(test.wantRepos))
			for i, want := range test.wantRepos {
				require.Equal(t, want.name, repos[i].Name)
				require.Equal(t, want.defaultBranch, repos[i].DefaultBranch)
			}
			for i, want := range test.wantWarnings {
				require.ErrorContains(t, warnings[i], want)
			}
		})
	}
}

func TestFormatRepositoryListWarnings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		warnings api.GraphQlErrors
		want     string
	}{
		{
			name: "with path",
			warnings: api.NewGraphQlErrors([]json.RawMessage{
				mustRawJSON(t, graphqlError("failed to resolve HEAD for github.com/sourcegraph/broken", "repositories", "nodes", 1, "defaultBranch")),
			}),
			want: `warnings: 1 errors during listing
repositories.nodes[1].defaultBranch - failed to resolve HEAD for github.com/sourcegraph/broken
{
  "message": "failed to resolve HEAD for github.com/sourcegraph/broken",
  "path": [
    "repositories",
    "nodes",
    1,
    "defaultBranch"
  ]
}
`,
		},
		{
			name: "without path",
			warnings: api.NewGraphQlErrors([]json.RawMessage{
				mustRawJSON(t, graphqlError("listing timed out")),
			}),
			want: `warnings: 1 errors during listing
listing timed out
{
  "message": "listing timed out"
}
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, test.want, formatRepositoryListWarnings(test.warnings))
		})
	}
}

func runListRepositories(t *testing.T, response map[string]any) ([]Repository, api.GraphQlErrors, error) {
	t.Helper()

	client := new(mockapi.Client)
	request := &mockapi.Request{Response: mustJSON(t, response)}

	client.On("NewRequest", mock.Anything, mock.Anything).Return(request).Once()
	request.On("DoRaw", context.Background(), mock.Anything).Return(true, nil).Once()

	repos, warnings, err := listRepositories(context.Background(), client, reposListOptions{
		first:      1000,
		cloned:     true,
		notCloned:  true,
		indexed:    true,
		notIndexed: false,
		orderBy:    "REPOSITORY_NAME",
	})

	client.AssertExpectations(t)
	request.AssertExpectations(t)

	return repos, warnings, err
}

func listRepositoriesResponse(nodes []map[string]any, graphqlErrors ...map[string]any) map[string]any {
	response := map[string]any{
		"data": map[string]any{
			"repositories": map[string]any{
				"nodes": nodes,
			},
		},
	}
	if len(graphqlErrors) > 0 {
		response["errors"] = graphqlErrors
	}
	return response
}

func repositoryNode(name string, defaultBranch any) map[string]any {
	return map[string]any{
		"name":          name,
		"defaultBranch": defaultBranch,
	}
}

func gitRefNode(name, displayName string) map[string]any {
	return map[string]any{
		"name":        name,
		"displayName": displayName,
	}
}

func graphqlError(message string, path ...any) map[string]any {
	err := map[string]any{"message": message}
	if len(path) > 0 {
		err["path"] = path
	}
	return err
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

func mustRawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()

	return json.RawMessage(mustJSON(t, v))
}
