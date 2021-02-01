package campaigns

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"github.com/sourcegraph/src-cli/internal/api"
	"github.com/sourcegraph/src-cli/internal/campaigns/graphql"
)

// RepoFetcher abstracts the process of retrieving an archive for the given
// repository.
type RepoFetcher interface {
	// MarkForLaterUse needs to be called before Fetch so that RepoFetcher can
	// register how many tasks will want to use the same archive and not delete
	// it prematurely.
	MarkForLaterUse(*graphql.Repository)

	// Fetch must retrieve the given repository and return it as a RepoZip.
	// This will generally imply that the file should be written to a temporary
	// location on the filesystem.
	Fetch(context.Context, *graphql.Repository) (RepoZip, error)
}

// repoFetcher is the concrete implementation of the RepoFetcher interface used
// outside of tests.
type repoFetcher struct {
	client     api.Client
	dir        string
	deleteZips bool

	zipsMu sync.Mutex
	zips   map[string]*repoZip
}

var _ RepoFetcher = &repoFetcher{}

func (rf *repoFetcher) zipFor(repo *graphql.Repository) *repoZip {
	rf.zipsMu.Lock()
	defer rf.zipsMu.Unlock()

	if rf.zips == nil {
		rf.zips = make(map[string]*repoZip)
	}

	path := filepath.Join(rf.dir, repo.Slug()+".zip")
	zip, ok := rf.zips[path]
	if !ok {
		zip = &repoZip{path: path, fetcher: rf}
		rf.zips[path] = zip
	}
	return zip
}

func (rf *repoFetcher) MarkForLaterUse(repo *graphql.Repository) {
	zip := rf.zipFor(repo)
	zip.mu.Lock()
	defer zip.mu.Unlock()

	zip.marks += 1
}

func (rf *repoFetcher) Fetch(ctx context.Context, repo *graphql.Repository) (RepoZip, error) {
	zip := rf.zipFor(repo)
	zip.mu.Lock()
	defer zip.mu.Unlock()

	// Someone already fetched it
	if zip.references > 0 {
		zip.references += 1
		zip.marks -= 1
		return zip, nil
	}

	exists, err := fileExists(zip.path)
	if err != nil {
		return nil, err
	}

	if !exists {
		// Unlike the mkdirAll() calls elsewhere in this file, this is only
		// giving us a temporary place on the filesystem to keep the archive.
		// Since it's never mounted into the containers being run, we can keep
		// these directories 0700 without issue.
		if err := os.MkdirAll(filepath.Dir(zip.path), 0700); err != nil {
			return nil, err
		}

		err = fetchRepositoryArchive(ctx, rf.client, repo, zip.path)
		if err != nil {
			// If the context got cancelled, or we ran out of disk space, or ...
			// while we were downloading the file, we remove the partially
			// downloaded file.
			os.Remove(zip.path)

			return nil, errors.Wrap(err, "fetching ZIP archive")
		}
	}

	zip.references += 1
	zip.marks -= 1
	return zip, nil
}

// RepoZip implementations represent a downloaded repository archive.
type RepoZip interface {
	// Close must finalise the downloaded archive. If one or more temporary
	// files were created, they should be deleted here.
	Close() error

	// Path must return the path to the archive on the filesystem.
	Path() string
}

var _ RepoZip = &repoZip{}

// repoZip is the concrete implementation of the RepoZip interface used outside
// of tests.
type repoZip struct {
	path    string
	fetcher *repoFetcher

	mu sync.Mutex
	// references is the number of *active* tasks that currently use the archive.
	references int
	// marks is the number of tasks that *will* make use of the archive.
	marks int
}

func (rz *repoZip) Close() error {
	rz.mu.Lock()
	defer rz.mu.Unlock()

	rz.references -= 1
	if rz.references == 0 && rz.marks == 0 && rz.fetcher.deleteZips {
		return os.Remove(rz.path)
	}

	return nil
}

func (rz *repoZip) Path() string {
	return rz.path
}

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
