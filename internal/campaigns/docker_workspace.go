package campaigns

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/campaigns/graphql"
)

type dockerWorkspaceCreator struct {
	dir string
}

var _ WorkspaceCreator = &dockerWorkspaceCreator{}

func (w *dockerWorkspaceCreator) Create(ctx context.Context, repo *graphql.Repository) (Workspace, error) {
	return nil, nil
}
