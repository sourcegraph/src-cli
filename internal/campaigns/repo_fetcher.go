package campaigns

import (
	"context"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/campaigns/graphql"
)

type RepoFetcher interface {
	Fetch(context.Context, *graphql.Repository) (RepoZip, error)
}

type repoFetcher struct {
	client     api.Client
	dir        string
	deleteZips bool
}

var _ RepoFetcher = &repoFetcher{}

type RepoZip interface {
	Close() error
	Path() string
}

type repoZip struct {
	path    string
	fetcher *repoFetcher
}

var _ RepoZip = &repoZip{}

func (rf *repoFetcher) Fetch(ctx context.Context, repo *graphql.Repository) (RepoZip, error) {
	path := localRepositoryZipArchivePath(rf.dir, repo)

	exists, err := fileExists(path)
	if err != nil {
		return nil, err
	}

	if !exists {
		// Unlike the mkdirAll() calls elsewhere in this file, this is only
		// giving us a temporary place on the filesystem to keep the archive.
		// Since it's never mounted into the containers being run, we can keep
		// these directories 0700 without issue.
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}

		err = fetchRepositoryArchive(ctx, rf.client, repo, path)
		if err != nil {
			// If the context got cancelled, or we ran out of disk space, or ...
			// while we were downloading the file, we remove the partially
			// downloaded file.
			os.Remove(path)

			return nil, errors.Wrap(err, "fetching ZIP archive")
		}
	}

	return &repoZip{
		path:    path,
		fetcher: rf,
	}, nil
}

func (rz *repoZip) Close() error {
	if rz.fetcher.deleteZips {
		return os.Remove(rz.path)
	}
	return nil
}

func (rz *repoZip) Path() string {
	return rz.path
}
