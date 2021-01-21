package campaigns

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/sourcegraph/src-cli/internal/campaigns/graphql"
)

type dockerBindWorkspaceCreator struct {
	dir string
}

var _ WorkspaceCreator = &dockerBindWorkspaceCreator{}

func (wc *dockerBindWorkspaceCreator) Create(ctx context.Context, repo *graphql.Repository, zip string) (Workspace, error) {
	w, err := wc.unzipToWorkspace(ctx, repo, zip)
	if err != nil {
		return nil, errors.Wrap(err, "unzipping the repository")
	}

	return w, errors.Wrap(wc.prepareGitRepo(ctx, w), "preparing local git repo")
}

func (*dockerBindWorkspaceCreator) prepareGitRepo(ctx context.Context, w *dockerBindWorkspace) error {
	if _, err := runGitCmd(ctx, w.dir, "init"); err != nil {
		return errors.Wrap(err, "git init failed")
	}

	// --force because we want previously "gitignored" files in the repository
	if _, err := runGitCmd(ctx, w.dir, "add", "--force", "--all"); err != nil {
		return errors.Wrap(err, "git add failed")
	}
	if _, err := runGitCmd(ctx, w.dir, "commit", "--quiet", "--all", "--allow-empty", "-m", "src-action-exec"); err != nil {
		return errors.Wrap(err, "git commit failed")
	}

	return nil
}

func (wc *dockerBindWorkspaceCreator) unzipToWorkspace(ctx context.Context, repo *graphql.Repository, zip string) (*dockerBindWorkspace, error) {
	prefix := "workspace-" + repo.Slug()
	workspace, err := unzipToTempDir(ctx, zip, wc.dir, prefix)
	if err != nil {
		return nil, errors.Wrap(err, "unzipping the ZIP archive")
	}

	return &dockerBindWorkspace{dir: workspace}, nil
}

type dockerBindWorkspace struct {
	dir string
}

var _ Workspace = &dockerBindWorkspace{}

func (w *dockerBindWorkspace) Close(ctx context.Context) error {
	return os.RemoveAll(w.dir)
}

func (w *dockerBindWorkspace) DockerRunOpts(ctx context.Context, target string) ([]string, error) {
	return []string{
		"--mount",
		fmt.Sprintf("type=bind,source=%s,target=%s", w.dir, target),
	}, nil
}

func (w *dockerBindWorkspace) WorkDir() *string { return &w.dir }

func (w *dockerBindWorkspace) Changes(ctx context.Context) (*StepChanges, error) {
	if _, err := runGitCmd(ctx, w.dir, "add", "--all"); err != nil {
		return nil, errors.Wrap(err, "git add failed")
	}

	statusOut, err := runGitCmd(ctx, w.dir, "status", "--porcelain")
	if err != nil {
		return nil, errors.Wrap(err, "git status failed")
	}

	changes, err := parseGitStatus(statusOut)
	if err != nil {
		return nil, errors.Wrap(err, "parsing git status output")
	}

	return &changes, nil
}

func (w *dockerBindWorkspace) Diff(ctx context.Context) ([]byte, error) {
	// As of Sourcegraph 3.14 we only support unified diff format.
	// That means we need to strip away the `a/` and `/b` prefixes with `--no-prefix`.
	// See: https://github.com/sourcegraph/sourcegraph/blob/82d5e7e1562fef6be5c0b17f18631040fd330835/enterprise/internal/campaigns/service.go#L324-L329
	//
	// Also, we need to add --binary so binary file changes are inlined in the patch.
	//
	return runGitCmd(ctx, w.dir, "diff", "--cached", "--no-prefix", "--binary")
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func unzipToTempDir(ctx context.Context, zipFile, tempDir, tempFilePrefix string) (string, error) {
	volumeDir, err := ioutil.TempDir(tempDir, tempFilePrefix)
	if err != nil {
		return "", err
	}

	if err := os.Chmod(volumeDir, 0777); err != nil {
		return "", err
	}

	return volumeDir, unzip(zipFile, volumeDir)
}

func localRepositoryZipArchivePath(dir string, repo *graphql.Repository) string {
	return filepath.Join(dir, fmt.Sprintf("%s-%s.zip", repo.Slug(), repo.Rev()))
}

func unzip(zipFile, dest string) error {
	r, err := zip.OpenReader(zipFile)
	if err != nil {
		return err
	}
	defer r.Close()

	outputBase := filepath.Clean(dest) + string(os.PathSeparator)

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: https://snyk.io/research/zip-slip-vulnerability#go
		if !strings.HasPrefix(fpath, outputBase) {
			return fmt.Errorf("%s: illegal file path", fpath)
		}

		if f.FileInfo().IsDir() {
			if err := mkdirAll(dest, f.Name, 0777); err != nil {
				return err
			}
			continue
		}

		if err := mkdirAll(dest, filepath.Dir(f.Name), 0777); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		// Since the container might not run as the same user, we need to ensure
		// that the file is globally writable. If the execute bit is normally
		// set on the zipped up file, let's ensure we propagate that to the
		// group and other permission bits too.
		if f.Mode()&0111 != 0 {
			if err := os.Chmod(outFile.Name(), 0777); err != nil {
				return err
			}
		} else {
			if err := os.Chmod(outFile.Name(), 0666); err != nil {
				return err
			}
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		cerr := outFile.Close()
		// Now we have safely closed everything that needs it, and can check errors
		if err != nil {
			return errors.Wrapf(err, "copying %q failed", f.Name)
		}
		if cerr != nil {
			return errors.Wrap(err, "closing output file failed")
		}

	}

	return nil
}

// Technically, this is a misnomer, since it might be a socket or block special,
// but errPathExistsAsNonDir is just ugly for an internal type.
type errPathExistsAsFile string

var _ error = errPathExistsAsFile("")

func (e errPathExistsAsFile) Error() string {
	return fmt.Sprintf("path already exists, but not as a directory: %s", string(e))
}

// mkdirAll is essentially os.MkdirAll(filepath.Join(base, path), perm), but
// applies the given permission regardless of the user's umask.
func mkdirAll(base, path string, perm os.FileMode) error {
	abs := filepath.Join(base, path)

	// Create the directory if it doesn't exist.
	st, err := os.Stat(abs)
	if err != nil {
		// It's expected that we'll get an error if the directory doesn't exist,
		// so let's check that it's of the type we expect.
		if !os.IsNotExist(err) {
			return err
		}

		// Now we're clear to create the directory.
		if err := os.MkdirAll(abs, perm); err != nil {
			return err
		}
	} else if !st.IsDir() {
		// The file/socket/whatever exists, but it's not a directory. That's
		// definitely going to be an issue.
		return errPathExistsAsFile(abs)
	}

	// If os.MkdirAll() was invoked earlier, then the permissions it set were
	// subject to the umask. Let's walk the directories we may or may not have
	// created and ensure their permissions look how we want.
	return ensureAll(base, path, perm)
}

// ensureAll ensures that all directories under path have the expected
// permissions.
func ensureAll(base, path string, perm os.FileMode) error {
	var errs *multierror.Error

	// In plain English: for each directory in the path parameter, we should
	// chmod that path to the permissions that are expected.
	acc := []string{base}
	for _, element := range strings.Split(path, string(os.PathSeparator)) {
		acc = append(acc, element)
		if err := os.Chmod(filepath.Join(acc...), perm); err != nil {
			errs = multierror.Append(errs, err)
		}
	}

	return errs.ErrorOrNil()
}
