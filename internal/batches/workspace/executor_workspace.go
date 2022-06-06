package workspace

import (
	"context"
	"os"

	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"

	"github.com/sourcegraph/src-cli/internal/batches/graphql"
	"github.com/sourcegraph/src-cli/internal/batches/repozip"
)

type executorWorkspaceCreator struct {
	Dir string
}

var _ Creator = &executorWorkspaceCreator{}

func (wc *executorWorkspaceCreator) Type() CreatorType { return CreatorTypeExecutor }

func (wc *executorWorkspaceCreator) Create(ctx context.Context, repo *graphql.Repository, steps []batcheslib.Step, archive repozip.Archive) (Workspace, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// We just use the docker bind workspace implementation here, it works well.
	return &dockerBindWorkspace{tempDir: wc.Dir, dir: wd}, nil
}
