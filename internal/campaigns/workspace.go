package campaigns

import (
	"context"

	"github.com/sourcegraph/src-cli/internal/campaigns/graphql"
)

type WorkspaceCreator interface {
	Create(context.Context, *graphql.Repository) (Workspace, error)
}

type Workspace interface {
	Close(ctx context.Context) error
	DockerRunOpts(ctx context.Context, target string) ([]string, error)
	Prepare(ctx context.Context) error

	Changes(ctx context.Context) (*StepChanges, error)
	Diff(ctx context.Context) ([]byte, error)
}
