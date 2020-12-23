package campaigns

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/campaigns/graphql"
)

// WorkspaceCreator implementations are used to create workspaces, which manage
// per-changeset persistent storage when executing campaign steps and are
// responsible for ultimately generating a diff.
type WorkspaceCreator interface {
	// Create should clone the given repository, and perform whatever other
	// initial setup is required.
	Create(context.Context, *graphql.Repository) (Workspace, error)
}

// Workspace implementations manage per-changeset storage when executing
// campaign step.
type Workspace interface {
	// Prepare is called once, immediately before the first step is executed.
	// Generally, this would perform whatever Git or other VCS setup is required
	// to establish a base upon which to calculate changes later.
	Prepare(ctx context.Context) error

	// DockerRunOpts provides the options that should be given to `docker run`
	// in order to use this workspace. Generally, this will be a set of mount
	// options.
	DockerRunOpts(ctx context.Context, target string) ([]string, error)

	// Close is called once, after all steps have been executed and the diff has
	// been calculated and stored outside the workspace. Implementations should
	// delete the workspace when Close is called.
	Close(ctx context.Context) error

	// Changes is called after each step is executed, and should return the
	// cumulative file changes that have occurred since Prepare was called.
	Changes(ctx context.Context) (*StepChanges, error)

	// Diff should return the total diff for the workspace. This may be called
	// multiple times in the life of a workspace.
	Diff(ctx context.Context) ([]byte, error)
}

// We'll put some useful utility functions below here that tend to be reused
// across workspace implementations.

func fetchRepositoryArchive(ctx context.Context, client api.Client, repo *graphql.Repository, dest string) error {
	req, err := client.NewHTTPRequest(ctx, "GET", repositoryZipArchivePath(repo), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/zip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unable to fetch archive (HTTP %d from %s)", resp.StatusCode, req.URL.String())
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}

	return nil
}

func repositoryZipArchivePath(repo *graphql.Repository) string {
	return path.Join("", repo.Name+"@"+repo.BaseRef(), "-", "raw")
}
