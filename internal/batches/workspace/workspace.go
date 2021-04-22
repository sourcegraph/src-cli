package workspace

import (
	"context"
	"runtime"

	"github.com/sourcegraph/src-cli/internal/batches"
	"github.com/sourcegraph/src-cli/internal/batches/git"
	"github.com/sourcegraph/src-cli/internal/batches/graphql"
)

// Creator implementations are used to create workspaces, which manage
// per-changeset persistent storage when executing batch change steps and are
// responsible for ultimately generating a diff.
type Creator interface {
	// Create creates a new workspace for the given repository and archive file.
	Create(ctx context.Context, repo *graphql.Repository, steps []batches.Step, archive batches.RepoZip) (Workspace, error)

	// Type returns the CreatorType of the Creator.
	Type() CreatorType
}

// Workspace implementations manage per-changeset storage when executing batch
// change steps.
type Workspace interface {
	// DockerRunOpts provides the options that should be given to `docker run`
	// in order to use this workspace. Generally, this will be a set of mount
	// options.
	DockerRunOpts(ctx context.Context, target string) ([]string, error)

	// WorkDir allows workspaces to specify the working directory that should be
	// used when running Docker. If no specific working directory is needed,
	// then the function should return nil.
	WorkDir() *string

	// Close is called once, after all steps have been executed and the diff has
	// been calculated and stored outside the workspace. Implementations should
	// delete the workspace when Close is called.
	Close(ctx context.Context) error

	// Changes is called after each step is executed, and should return the
	// cumulative file changes that have occurred since Prepare was called.
	Changes(ctx context.Context) (*git.Changes, error)

	// Diff should return the total diff for the workspace. This may be called
	// multiple times in the life of a workspace.
	Diff(ctx context.Context) ([]byte, error)
}

type CreatorType int

const (
	CreatorTypeBind CreatorType = iota
	CreatorTypeVolume
)

func NewCreator(ctx context.Context, preference, cacheDir, tempDir string, steps []batches.Step) Creator {
	var workspaceType CreatorType
	if preference == "volume" {
		workspaceType = CreatorTypeVolume
	} else if preference == "bind" {
		workspaceType = CreatorTypeBind
	} else {
		workspaceType = BestCreatorType(ctx, steps)
	}

	if workspaceType == CreatorTypeVolume {
		return &dockerVolumeWorkspaceCreator{tempDir: tempDir}
	}
	return &dockerBindWorkspaceCreator{Dir: cacheDir}
}

// BestCreatorType determines the correct workspace creator type to use based
// on the environment and batch change to be executed.
func BestCreatorType(ctx context.Context, steps []batches.Step) CreatorType {
	// The basic theory here is that we have two options: bind and volume. Bind
	// is battle tested and always safe, but can be slow on non-Linux platforms
	// because bind mounts are slow. Volume is faster on those platforms, but
	// exposes users to UID mismatch issues they'd otherwise be insulated from
	// by the semantics of bind mounting on non-Linux platforms: specifically,
	// if you have a batch change with steps that run as UID 1000 and then UID
	// 2000, you'll get errors when the second step tries to write.

	// For the time being, we're only going to consider volume mode on Intel
	// macOS.
	if runtime.GOOS != "darwin" || runtime.GOARCH != "amd64" {
		return CreatorTypeBind
	}

	return detectBestCreatorType(ctx, steps)
}

func detectBestCreatorType(ctx context.Context, steps []batches.Step) CreatorType {
	// OK, so we're interested in volume mode, but we need to take its
	// shortcomings around mixed user environments into account.
	//
	// To do that, let's iterate over the Docker images that are going to be
	// used and get their default UID. This admittedly only gets us so far —
	// there's nothing stopping an adventurous user from running su directly in
	// their script, or running a setuid binary — but it should be a good enough
	// heuristic. (And, if we get this wrong, there's nothing stopping a user
	// from providing a workspace type explicitly with the -workspace flag.)
	//
	// Once we have the UIDs, it's pretty simple: the moment we see more than
	// one UID, we should fall back to bind mode.
	//
	// In theory, we could make this more sensitive and complicated: a non-root
	// container that's followed by only root containers would actually be OK,
	// but let's keep it simple for now.
	var uid *int

	for _, step := range steps {
		ug, err := step.ImageUIDGID(ctx)
		if err != nil {
			// An error here likely indicates that `id` isn't available on the
			// path. That's OK: let's not make any assumptions at this point
			// about the image, and we'll default to the always safe option.
			return CreatorTypeBind
		}

		if uid == nil {
			uid = &ug.UID
		} else if *uid != ug.UID {
			return CreatorTypeBind
		}
	}

	return CreatorTypeVolume
}
